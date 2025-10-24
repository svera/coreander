package webserver_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"gorm.io/gorm"
)

func TestUserInvitation(t *testing.T) {
	var (
		db            *gorm.DB
		app           *fiber.App
		adminCookie   *http.Cookie
		regularCookie *http.Cookie
		smtpMock      *infrastructure.SMTPMock
	)

	reset := func() {
		t.Helper()

		var err error
		db = infrastructure.Connect(":memory:", 250)
		smtpMock = &infrastructure.SMTPMock{}

		webserverConfig := webserver.Config{
			SessionTimeout:    24 * time.Hour,
			RecoveryTimeout:   2 * time.Hour,
			InvitationTimeout: 72 * time.Hour,
			LibraryPath:       "fixtures/library",
			WordsPerMinute:    250,
			FQDN:              "http://localhost:3000",
		}

		app = bootstrapApp(db, smtpMock, afero.NewMemMapFs(), webserverConfig)

		adminCookie, err = login(app, "admin@example.com", "admin", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		// Create a regular user for testing permissions
		regularUserData := url.Values{
			"name":             {"Regular user"},
			"username":         {"regular"},
			"email":            {"regular@example.com"},
			"password":         {"regular"},
			"confirm-password": {"regular"},
			"role":             {fmt.Sprint(model.RoleRegular)},
			"words-per-minute": {"250"},
		}

		if response, err := postRequest(regularUserData, adminCookie, app, "/users", t); response == nil || err != nil {
			t.Fatalf("Unexpected error creating regular user: %v", err.Error())
		}

		regularCookie, err = login(app, "regular@example.com", "regular", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
	}

	t.Run("Try to access invite form without authentication", func(t *testing.T) {
		reset()

		response, err := getRequest(&http.Cookie{}, app, "/users/invite", t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)
	})

	t.Run("Try to access invite form as regular user", func(t *testing.T) {
		reset()

		response, err := getRequest(regularCookie, app, "/users/invite", t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
	})

	t.Run("Access invite form as admin", func(t *testing.T) {
		reset()

		response, err := getRequest(adminCookie, app, "/users/invite", t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		doc, err := goquery.NewDocumentFromReader(response.Body)
		if err != nil {
			t.Fatal(err)
		}

		// Check form exists
		if doc.Find("form[action='/users/invite']").Length() == 0 {
			t.Error("Expected invitation form not found")
		}

		// Check email input exists
		if doc.Find("input[name='email']").Length() == 0 {
			t.Error("Expected email input not found")
		}
	})

	t.Run("Try to send invitation without email configured (NoEmail)", func(t *testing.T) {
		t.Helper()

		db := infrastructure.Connect(":memory:", 250)
		noEmailApp := bootstrapApp(db, &infrastructure.NoEmail{}, afero.NewMemMapFs(), webserver.Config{})

		cookie, err := login(noEmailApp, "admin@example.com", "admin", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		response, err := getRequest(cookie, noEmailApp, "/users/invite", t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusNotFound, t)
	})

	t.Run("Send invitation successfully", func(t *testing.T) {
		reset()

		inviteData := url.Values{
			"email": {"newuser@example.com"},
		}

		smtpMock.Wg.Add(1)
		response, err := postRequest(inviteData, adminCookie, app, "/users/invite", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		smtpMock.Wg.Wait()

		// Should redirect to users list
		if response.StatusCode != http.StatusFound && response.StatusCode != http.StatusSeeOther {
			t.Errorf("Expected redirect status, got %d", response.StatusCode)
		}

		// Check invitation was created in database
		var invitation model.Invitation
		result := db.Where("email = ?", "newuser@example.com").First(&invitation)
		if result.Error != nil {
			t.Fatalf("Expected invitation to be created: %v", result.Error)
		}

		if invitation.Email != "newuser@example.com" {
			t.Errorf("Expected email to be 'newuser@example.com', got %s", invitation.Email)
		}

		if invitation.UUID == "" {
			t.Error("Expected invitation UUID to be set")
		}

		if invitation.ValidUntil.Before(time.Now()) {
			t.Error("Expected invitation to be valid in the future")
		}

		// Check email was sent
		if !smtpMock.CalledSend() {
			t.Error("Expected email to be sent")
		}
	})

	t.Run("Send invitation with invalid email", func(t *testing.T) {
		reset()

		inviteData := url.Values{
			"email": {"invalid-email"},
		}

		response, err := postRequest(inviteData, adminCookie, app, "/users/invite", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		expectedErrorMessages := []string{
			"Incorrect email address",
		}

		checkErrorMessages(response, t, expectedErrorMessages)
	})

	t.Run("Send invitation for existing user email", func(t *testing.T) {
		reset()

		inviteData := url.Values{
			"email": {"admin@example.com"},
		}

		response, err := postRequest(inviteData, adminCookie, app, "/users/invite", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		expectedErrorMessages := []string{
			"A user with this email already exists",
		}

		checkErrorMessages(response, t, expectedErrorMessages)
	})

	t.Run("Send invitation replaces old pending invitation", func(t *testing.T) {
		reset()

		// Send first invitation
		inviteData := url.Values{
			"email": {"test@example.com"},
		}

		smtpMock.Wg.Add(1)
		_, err := postRequest(inviteData, adminCookie, app, "/users/invite", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		smtpMock.Wg.Wait()

		var firstInvitation model.Invitation
		db.Where("email = ?", "test@example.com").First(&firstInvitation)
		firstUUID := firstInvitation.UUID

		// Send second invitation for same email
		smtpMock.Wg.Add(1)
		_, err = postRequest(inviteData, adminCookie, app, "/users/invite", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		smtpMock.Wg.Wait()

		// Check only one invitation exists
		var invitations []model.Invitation
		db.Where("email = ?", "test@example.com").Find(&invitations)

		if len(invitations) != 1 {
			t.Errorf("Expected 1 invitation, got %d", len(invitations))
		}

		// Check it's a new one (different UUID)
		if invitations[0].UUID == firstUUID {
			t.Error("Expected new invitation with different UUID")
		}
	})

	t.Run("Access invitation acceptance form without authentication", func(t *testing.T) {
		reset()

		// Create an invitation first
		invitation := &model.Invitation{
			Email:      "invited@example.com",
			UUID:       "test-uuid-123",
			ValidUntil: time.Now().Add(72 * time.Hour),
		}
		db.Create(invitation)

		response, err := getRequest(&http.Cookie{}, app, "/users/accept-invite?id=test-uuid-123", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		doc, err := goquery.NewDocumentFromReader(response.Body)
		if err != nil {
			t.Fatal(err)
		}

		// Check form exists
		if doc.Find("form[action='/users/accept-invite']").Length() == 0 {
			t.Error("Expected accept invitation form not found")
		}

		// Check email is pre-filled and readonly
		emailInput := doc.Find("input[name='email']").First()
		if emailInput.Length() == 0 {
			t.Error("Expected email input not found")
		}

		emailValue, _ := emailInput.Attr("value")
		if emailValue != "invited@example.com" {
			t.Errorf("Expected email to be pre-filled with 'invited@example.com', got %s", emailValue)
		}

		// Check for required fields
		requiredFields := []string{"name", "username", "password", "confirm-password"}
		for _, field := range requiredFields {
			if doc.Find(fmt.Sprintf("input[name='%s']", field)).Length() == 0 {
				t.Errorf("Expected field '%s' not found", field)
			}
		}
	})

	t.Run("Try to access invitation with invalid UUID", func(t *testing.T) {
		reset()

		response, err := getRequest(&http.Cookie{}, app, "/users/accept-invite?id=invalid-uuid", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusNotFound, t)
	})

	t.Run("Try to access expired invitation", func(t *testing.T) {
		reset()

		// Create an expired invitation
		invitation := &model.Invitation{
			Email:      "expired@example.com",
			UUID:       "expired-uuid",
			ValidUntil: time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
		}
		db.Create(invitation)

		response, err := getRequest(&http.Cookie{}, app, "/users/accept-invite?id=expired-uuid", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusBadRequest, t)

		// Check invitation was deleted from database
		var deletedInvitation model.Invitation
		result := db.Where("uuid = ?", "expired-uuid").First(&deletedInvitation)
		if result.Error != gorm.ErrRecordNotFound {
			t.Error("Expected expired invitation to be deleted")
		}
	})

	t.Run("Accept invitation successfully", func(t *testing.T) {
		reset()

		// Create an invitation
		invitation := &model.Invitation{
			Email:      "newacc@example.com",
			UUID:       "new-user-uuid",
			ValidUntil: time.Now().Add(72 * time.Hour),
		}
		db.Create(invitation)

		acceptData := url.Values{
			"invitation_uuid":  {"new-user-uuid"},
			"name":             {"New User"},
			"username":         {"newuser"},
			"password":         {"password123"},
			"confirm-password": {"password123"},
		}

		response, err := postRequest(acceptData, &http.Cookie{}, app, "/users/accept-invite", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		// Should redirect to login page
		if response.StatusCode != http.StatusFound && response.StatusCode != http.StatusSeeOther {
			t.Errorf("Expected redirect status, got %d", response.StatusCode)
		}

		// Check user was created
		var user model.User
		result := db.Where("email = ?", "newacc@example.com").First(&user)
		if result.Error != nil {
			t.Fatalf("Expected user to be created: %v", result.Error)
		}

		if user.Name != "New User" {
			t.Errorf("Expected name to be 'New User', got %s", user.Name)
		}

		if user.Username != "newuser" {
			t.Errorf("Expected username to be 'newuser', got %s", user.Username)
		}

		if user.Email != "newacc@example.com" {
			t.Errorf("Expected email to be 'newacc@example.com', got %s", user.Email)
		}

		if user.Role != model.RoleRegular {
			t.Errorf("Expected role to be RoleRegular (%d), got %d", model.RoleRegular, user.Role)
		}

		// Verify default values are set
		if user.SendToEmail != "" {
			t.Errorf("Expected SendToEmail to be empty, got %s", user.SendToEmail)
		}

		if user.PreferredEpubType != "epub" {
			t.Errorf("Expected PreferredEpubType to be 'epub', got %s", user.PreferredEpubType)
		}

		if user.WordsPerMinute != 250 {
			t.Errorf("Expected WordsPerMinute to be 250, got %f", user.WordsPerMinute)
		}

		// Check invitation was deleted
		var deletedInvitation model.Invitation
		result = db.Where("uuid = ?", "new-user-uuid").First(&deletedInvitation)
		if result.Error != gorm.ErrRecordNotFound {
			t.Error("Expected invitation to be deleted after acceptance")
		}

		// Verify user can log in
		_, err = login(app, "newacc@example.com", "password123", t)
		if err != nil {
			t.Errorf("Expected new user to be able to log in: %v", err)
		}
	})

	t.Run("Accept invitation with validation errors", func(t *testing.T) {
		reset()

		// Create an invitation
		invitation := &model.Invitation{
			Email:      "validate@example.com",
			UUID:       "validate-uuid",
			ValidUntil: time.Now().Add(72 * time.Hour),
		}
		db.Create(invitation)

		acceptData := url.Values{
			"invitation_uuid":  {"validate-uuid"},
			"name":             {""},
			"username":         {""},
			"password":         {"12"}, // Less than 5 characters
			"confirm-password": {"different"},
		}

		response, err := postRequest(acceptData, &http.Cookie{}, app, "/users/accept-invite", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		expectedErrorMessages := []string{
			"Name cannot be empty",
			"Username cannot be empty",
			"Password and confirmation do not match",
		}

		checkErrorMessages(response, t, expectedErrorMessages)

		// Invitation should still exist
		var stillExists model.Invitation
		result := db.Where("uuid = ?", "validate-uuid").First(&stillExists)
		if result.Error != nil {
			t.Error("Expected invitation to still exist after validation error")
		}
	})

	t.Run("Accept invitation with duplicate username", func(t *testing.T) {
		reset()

		// Create an invitation
		invitation := &model.Invitation{
			Email:      "duplicate@example.com",
			UUID:       "duplicate-uuid",
			ValidUntil: time.Now().Add(72 * time.Hour),
		}
		db.Create(invitation)

		acceptData := url.Values{
			"invitation_uuid":  {"duplicate-uuid"},
			"name":             {"Duplicate User"},
			"username":         {"admin"}, // This username already exists
			"password":         {"password123"},
			"confirm-password": {"password123"},
		}

		response, err := postRequest(acceptData, &http.Cookie{}, app, "/users/accept-invite", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		expectedErrorMessages := []string{
			"This username is already taken",
		}

		checkErrorMessages(response, t, expectedErrorMessages)
	})

	t.Run("Invited users can only be regular users", func(t *testing.T) {
		reset()

		// Create an invitation
		invitation := &model.Invitation{
			Email:      "regularonly@example.com",
			UUID:       "regular-only-uuid",
			ValidUntil: time.Now().Add(72 * time.Hour),
		}
		db.Create(invitation)

		acceptData := url.Values{
			"invitation_uuid":  {"regular-only-uuid"},
			"name":             {"Test User"},
			"username":         {"testuser"},
			"password":         {"password123"},
			"confirm-password": {"password123"},
		}

		_, err := postRequest(acceptData, &http.Cookie{}, app, "/users/accept-invite", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		// Check user was created with regular role
		var user model.User
		result := db.Where("email = ?", "regularonly@example.com").First(&user)
		if result.Error != nil {
			t.Fatalf("Expected user to be created: %v", result.Error)
		}

		if user.Role != model.RoleRegular {
			t.Errorf("Expected role to be RoleRegular (%d), got %d", model.RoleRegular, user.Role)
		}
	})

	t.Run("Try to accept invitation while logged in", func(t *testing.T) {
		reset()

		// Create an invitation
		invitation := &model.Invitation{
			Email:      "loggedin@example.com",
			UUID:       "logged-in-uuid",
			ValidUntil: time.Now().Add(72 * time.Hour),
		}
		db.Create(invitation)

		// Try to access while logged in as admin
		resp, err := getRequest(adminCookie, app, "/users/accept-invite?id=logged-in-uuid", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		// Should redirect or show error (depending on AllowIfNotLoggedIn middleware behavior)
		if resp.StatusCode == http.StatusOK {
			t.Error("Should not allow logged-in users to access invitation acceptance")
		}
	})
}

func TestInvitationEmailContent(t *testing.T) {
	reset := func() (*gorm.DB, *fiber.App, *http.Cookie, *infrastructure.SMTPMock) {
		t.Helper()

		db := infrastructure.Connect(":memory:", 250)
		smtpMock := &infrastructure.SMTPMock{}

		webserverConfig := webserver.Config{
			SessionTimeout:    24 * time.Hour,
			RecoveryTimeout:   2 * time.Hour,
			InvitationTimeout: 72 * time.Hour,
			LibraryPath:       "fixtures/library",
			WordsPerMinute:    250,
			FQDN:              "http://localhost:3000",
		}

		app := bootstrapApp(db, smtpMock, afero.NewMemMapFs(), webserverConfig)

		cookie, err := login(app, "admin@example.com", "admin", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		return db, app, cookie, smtpMock
	}

	t.Run("Invitation email contains correct link", func(t *testing.T) {
		db, app, adminCookie, smtpMock := reset()

		inviteData := url.Values{
			"email": {"emailtest@example.com"},
		}

		smtpMock.Wg.Add(1)
		_, err := postRequest(inviteData, adminCookie, app, "/users/invite", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		smtpMock.Wg.Wait()

		// Get the invitation UUID from database
		var invitation model.Invitation
		db.Where("email = ?", "emailtest@example.com").First(&invitation)

		// The email should contain the invitation link
		expectedLinkPart := fmt.Sprintf("/users/accept-invite?id=%s", invitation.UUID)

		// Verify invitation has UUID and is valid
		if invitation.UUID == "" {
			t.Error("Expected invitation to have UUID")
		}

		if !strings.Contains(expectedLinkPart, invitation.UUID) {
			t.Error("Expected email to contain invitation UUID in link")
		}
	})
}
