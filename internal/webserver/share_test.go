package webserver_test

import (
	"encoding/json"
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

func TestShareRecipientsExcludesSessionUser(t *testing.T) {
	_, app, adminCookie := setupShareTest(t, webserver.Config{}, &infrastructure.NoEmail{})

	createUser(t, app, adminCookie, userFixture{
		name:     "Regular user",
		username: "regular",
		email:    "regular@example.com",
		password: "regular",
	})

	response, err := getRequest(adminCookie, app, "/users/share-recipients", t)
	if response == nil || err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	var recipients []struct {
		Username string `json:"username"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(response.Body).Decode(&recipients); err != nil {
		t.Fatalf("Unexpected error decoding response: %v", err)
	}

	hasAdmin := false
	hasRegular := false
	for _, recipient := range recipients {
		if recipient.Username == "admin" {
			hasAdmin = true
		}
		if recipient.Username == "regular" {
			hasRegular = true
		}
	}

	if hasAdmin {
		t.Error("Expected share recipients to exclude session user")
	}
	if !hasRegular {
		t.Error("Expected share recipients to include other users")
	}
}

func TestShareCommentIsTruncated(t *testing.T) {
	webserverConfig := webserver.Config{
		ShareCommentMaxSize: 280, // Use default value
		ShareMaxRecipients:  10,
		LibraryPath:         "fixtures/library",
		WordsPerMinute:      250,
		SessionTimeout:      24 * time.Hour,
		RecoveryTimeout:     2 * time.Hour,
		MinPasswordLength:  5,
	}
	db, app, adminCookie, smtpMock := setupShareTestWithSMTP(t, webserverConfig)
	createUser(t, app, adminCookie, userFixture{
		name:     "Regular user",
		username: "regular",
		email:    "regular@example.com",
		password: "regular",
	})

	longComment := strings.Repeat("a", 300)
	shareData := url.Values{
		"recipients": {"regular"},
		"comment":    {longComment},
	}
	smtpMock.Wg.Add(1)
	response, err := postRequest(shareData, adminCookie, app, "/documents/john-doe-test-epub/share", t)
	if response == nil || err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	regularUser := model.User{}
	db.Where("email = ?", "regular@example.com").First(&regularUser)

	highlight := model.Highlight{}
	db.Where("user_id = ?", regularUser.ID).First(&highlight)
	if highlight.Path == "" {
		t.Fatal("Expected share highlight to be created")
	}

	expected := string([]rune(longComment)[:webserverConfig.ShareCommentMaxSize])
	if highlight.Comment != expected {
		t.Errorf("Expected comment to be %d characters, got %d", len(expected), len(highlight.Comment))
	}
}

func TestShareFailsWhenSenderIsPrivate(t *testing.T) {
	webserverConfig := webserver.Config{
		ShareMaxRecipients: 10,
		LibraryPath:        "fixtures/library",
		WordsPerMinute:     250,
		SessionTimeout:     24 * time.Hour,
		RecoveryTimeout:    2 * time.Hour,
		MinPasswordLength:  5,
	}
	db, app, adminCookie, smtpMock := setupShareTestWithSMTP(t, webserverConfig)
	setPrivateProfile(t, db, "admin@example.com", true)
	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	createUser(t, app, adminCookie, userFixture{
		name:     "Regular user",
		username: "regular",
		email:    "regular@example.com",
		password: "regular",
	})

	shareData := url.Values{
		"recipients": {"regular"},
	}
	smtpMock.Wg.Add(1)
	response, err := postRequest(shareData, adminCookie, app, "/documents/john-doe-test-epub/share", t)
	if response == nil || err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("Expected status %d, received %d", http.StatusForbidden, response.StatusCode)
	}

	regularUser := model.User{}
	db.Where("email = ?", "regular@example.com").First(&regularUser)

	var count int64
	db.Model(&model.Highlight{}).Where("user_id = ?", regularUser.ID).Count(&count)
	if count != 0 {
		t.Fatal("Expected no shares to be created for private sender")
	}
}

func TestShareFailsWhenRecipientIsPrivate(t *testing.T) {
	webserverConfig := webserver.Config{
		ShareMaxRecipients: 10,
		LibraryPath:        "fixtures/library",
		WordsPerMinute:     250,
		SessionTimeout:     24 * time.Hour,
		RecoveryTimeout:    2 * time.Hour,
		MinPasswordLength:  5,
	}
	db, app, adminCookie, smtpMock := setupShareTestWithSMTP(t, webserverConfig)
	createUser(t, app, adminCookie, userFixture{
		name:     "Regular user",
		username: "regular",
		email:    "regular@example.com",
		password: "regular",
	})
	setPrivateProfile(t, db, "regular@example.com", true)

	shareData := url.Values{
		"recipients": {"regular"},
	}
	smtpMock.Wg.Add(1)
	response, err := postRequest(shareData, adminCookie, app, "/documents/john-doe-test-epub/share", t)
	if response == nil || err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected status %d, received %d", http.StatusBadRequest, response.StatusCode)
	}

	regularUser := fetchUserByEmail(t, db, "regular@example.com")
	var count int64
	db.Model(&model.Highlight{}).Where("user_id = ?", regularUser.ID).Count(&count)
	if count != 0 {
		t.Fatal("Expected no shares to be created for private recipient")
	}
}

func TestShareNotAvailableWhenEmailServiceNotConfigured(t *testing.T) {
	_, app, adminCookie := setupShareTest(t, webserver.Config{}, &infrastructure.NoEmail{})
	createUser(t, app, adminCookie, userFixture{
		name:     "Regular user",
		username: "regular",
		email:    "regular@example.com",
		password: "regular",
	})

	// Test that share endpoint returns 404 when email service is not configured
	shareData := url.Values{
		"recipients": {"regular"},
	}
	response, err := postRequest(shareData, adminCookie, app, "/documents/john-doe-test-epub/share", t)
	if response == nil || err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("Expected status %d, received %d", http.StatusNotFound, response.StatusCode)
	}

	// Test that share UI elements are not present in document detail page
	response, err = getRequest(adminCookie, app, "/documents/john-doe-test-epub", t)
	if response == nil || err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatalf("Unexpected error parsing HTML: %v", err)
	}

	// Check that share modal is not present
	if doc.Find("#share-modal-john-doe-test-epub").Length() > 0 {
		t.Error("Expected share modal to not be present when email service is not configured")
	}

	// Check that share button is not present in dropdown
	shareButtons := doc.Find(".bi-share-fill")
	if shareButtons.Length() > 0 {
		t.Error("Expected share buttons to not be present when email service is not configured")
	}
}

func TestShareFailsWhenRecipientLimitExceeded(t *testing.T) {
	webserverConfig := webserver.Config{
		ShareMaxRecipients: 3,
		LibraryPath:        "fixtures/library",
		WordsPerMinute:     250,
		SessionTimeout:     24 * time.Hour,
		RecoveryTimeout:    2 * time.Hour,
		MinPasswordLength: 5,
	}
	_, app, adminCookie, smtpMock := setupShareTestWithSMTP(t, webserverConfig)

	// Create multiple users
	users := []struct {
		name     string
		username string
		email    string
	}{
		{"User 1", "user1", "user1@example.com"},
		{"User 2", "user2", "user2@example.com"},
		{"User 3", "user3", "user3@example.com"},
		{"User 4", "user4", "user4@example.com"},
	}

	for _, user := range users {
		createUser(t, app, adminCookie, userFixture{
			name:     user.name,
			username: user.username,
			email:    user.email,
			password: "password",
		})
	}

	// Test sharing with exactly the limit (should succeed)
	shareData := url.Values{
		"recipients": {"user1,user2,user3"},
	}
	smtpMock.Wg.Add(1)
	response, err := postRequest(shareData, adminCookie, app, "/documents/john-doe-test-epub/share", t)
	if response == nil || err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d for limit-compliant share, received %d", http.StatusOK, response.StatusCode)
	}

	// Test sharing with more than the limit (should fail)
	shareData = url.Values{
		"recipients": {"user1,user2,user3,user4"},
	}
	smtpMock.Wg.Add(1)
	response, err = postRequest(shareData, adminCookie, app, "/documents/john-doe-test-epub/share", t)
	if response == nil || err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected status %d for exceeding recipient limit, received %d", http.StatusBadRequest, response.StatusCode)
	}
}

func TestShareCommentSizeLimit(t *testing.T) {
	webserverConfig := webserver.Config{
		ShareCommentMaxSize: 100,
		ShareMaxRecipients:  10,
		LibraryPath:         "fixtures/library",
		WordsPerMinute:      250,
		SessionTimeout:      24 * time.Hour,
		RecoveryTimeout:     2 * time.Hour,
		MinPasswordLength:  5,
	}
	db, app, adminCookie, smtpMock := setupShareTestWithSMTP(t, webserverConfig)
	createUser(t, app, adminCookie, userFixture{
		name:     "Regular user",
		username: "regular",
		email:    "regular@example.com",
		password: "regular",
	})

	// Create a second user for the second test
	createUser(t, app, adminCookie, userFixture{
		name:     "Second user",
		username: "second",
		email:    "second@example.com",
		password: "second",
	})

	// Test with comment exactly at limit
	exactLimitComment := strings.Repeat("a", webserverConfig.ShareCommentMaxSize)
	shareData := url.Values{
		"recipients": {"regular"},
		"comment":    {exactLimitComment},
	}
	smtpMock.Wg.Add(1)
	response, err := postRequest(shareData, adminCookie, app, "/documents/john-doe-test-epub/share", t)
	if response == nil || err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	regularUser := model.User{}
	db.Where("email = ?", "regular@example.com").First(&regularUser)

	highlight := model.Highlight{}
	db.Where("user_id = ?", regularUser.ID).First(&highlight)
	if highlight.Comment != exactLimitComment {
		t.Errorf("Expected comment to be exactly %d characters, got %d", webserverConfig.ShareCommentMaxSize, len(highlight.Comment))
	}

	// Test with comment exceeding limit (should be truncated) - use different recipient
	longComment := strings.Repeat("b", webserverConfig.ShareCommentMaxSize+50)
	shareData = url.Values{
		"recipients": {"second"},
		"comment":    {longComment},
	}
	smtpMock.Wg.Add(1)
	response, err = postRequest(shareData, adminCookie, app, "/documents/john-doe-test-epub/share", t)
	if response == nil || err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	secondUser := model.User{}
	db.Where("email = ?", "second@example.com").First(&secondUser)

	highlight = model.Highlight{}
	db.Where("user_id = ?", secondUser.ID).First(&highlight)
	expected := string([]rune(longComment)[:webserverConfig.ShareCommentMaxSize])
	if highlight.Comment != expected {
		t.Errorf("Expected comment to be truncated to %d characters, got %d. Expected: %q, Got: %q", webserverConfig.ShareCommentMaxSize, len(highlight.Comment), expected, highlight.Comment)
	}
	if len(highlight.Comment) != webserverConfig.ShareCommentMaxSize {
		t.Errorf("Expected comment length to be %d, got %d", webserverConfig.ShareCommentMaxSize, len(highlight.Comment))
	}
}

type userFixture struct {
	name     string
	username string
	email    string
	password string
}

func setupShareTest(t *testing.T, config webserver.Config, sender webserver.Sender) (*gorm.DB, *fiber.App, *http.Cookie) {
	t.Helper()
	db := infrastructure.Connect(":memory:", 250)
	app := bootstrapApp(db, sender, afero.NewOsFs(), config)
	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	return db, app, adminCookie
}

func setupShareTestWithSMTP(t *testing.T, config webserver.Config) (*gorm.DB, *fiber.App, *http.Cookie, *infrastructure.SMTPMock) {
	t.Helper()
	smtpMock := &infrastructure.SMTPMock{}
	db, app, adminCookie := setupShareTest(t, config, smtpMock)
	return db, app, adminCookie, smtpMock
}

func createUser(t *testing.T, app *fiber.App, adminCookie *http.Cookie, user userFixture) {
	t.Helper()
	regularUserData := url.Values{
		"name":             {user.name},
		"username":         {user.username},
		"email":            {user.email},
		"password":         {user.password},
		"confirm-password": {user.password},
		"role":             {fmt.Sprint(model.RoleRegular)},
		"words-per-minute": {"250"},
	}
	response, err := postRequest(regularUserData, adminCookie, app, "/users", t)
	if response == nil || err != nil {
		t.Fatalf("Unexpected error creating user %s: %v", user.username, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, received %d for user %s", http.StatusOK, response.StatusCode, user.username)
	}
}

func setPrivateProfile(t *testing.T, db *gorm.DB, email string, private bool) {
	t.Helper()
	user := fetchUserByEmail(t, db, email)
	if private {
		user.PrivateProfile = 1
	} else {
		user.PrivateProfile = 0
	}
	db.Save(&user)
}

func fetchUserByEmail(t *testing.T, db *gorm.DB, email string) model.User {
	t.Helper()
	user := model.User{}
	db.Where("email = ?", email).First(&user)
	if user.ID == 0 {
		t.Fatalf("Expected user with email %s to exist", email)
	}
	return user
}
