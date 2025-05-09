package webserver_test

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"gorm.io/gorm"
)

func TestUserManagement(t *testing.T) {
	var (
		db                *gorm.DB
		app               *fiber.App
		adminCookie       *http.Cookie
		adminUser         model.User
		regularUserData   url.Values
		regularUser       model.User
		regularUserCookie *http.Cookie
	)

	reset := func() {
		t.Helper()

		var err error
		db = infrastructure.Connect(":memory:", 250)
		app = bootstrapApp(db, &infrastructure.NoEmail{}, afero.NewMemMapFs(), webserver.Config{})

		adminUser = model.User{}
		db.Where("email = ?", "admin@example.com").First(&adminUser)

		adminCookie, err = login(app, "admin@example.com", "admin", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		regularUserData = url.Values{
			"name":             {"Regular user"},
			"username":         {"regular"},
			"email":            {"regular@example.com"},
			"password":         {"regular"},
			"confirm-password": {"regular"},
			"role":             {fmt.Sprint(model.RoleRegular)},
			"words-per-minute": {"250"},
		}

		response, err := postRequest(regularUserData, adminCookie, app, "/users", t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		regularUser = model.User{}
		db.Where("email = ?", "regular@example.com").First(&regularUser)

		regularUserCookie, err = login(app, "regular@example.com", "regular", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
	}

	t.Run("Try to add a user without an active session", func(t *testing.T) {
		reset()

		response, err := getRequest(&http.Cookie{}, app, "/users/new", t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)

		newUserData := url.Values{
			"name":             {"New user"},
			"username":         {"new"},
			"email":            {"new@example.com"},
			"password":         {"new"},
			"confirm-password": {"new"},
			"role":             {fmt.Sprint(model.RoleRegular)},
			"words-per-minute": {"250"},
		}

		response, err = postRequest(newUserData, &http.Cookie{}, app, "/users", t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)
	})

	t.Run("Try to add a user with an admin active session", func(t *testing.T) {
		reset()

		response, err := getRequest(adminCookie, app, "/users/new", t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		newUserData := url.Values{
			"name":             {"New user   "}, // Extra spaces to test trimming
			"username":         {"new"},
			"email":            {"new@example.com"},
			"send-to-email":    {"send@example.com"},
			"password":         {"new"},
			"confirm-password": {"new"},
			"role":             {fmt.Sprint(model.RoleRegular)},
			"words-per-minute": {"250"},
		}

		response, err = postRequest(newUserData, adminCookie, app, "/users", t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		mustRedirectToUsersList(response, t)

		var totalUsers int64
		var user model.User
		db.Last(&user).Count(&totalUsers)

		if user.Name != "New user" {
			t.Errorf("Expected name to be 'New user', got %s", user.Name)
		}

		if totalUsers != 3 {
			t.Errorf("Expected 3 users in the users list, got %d", totalUsers)
		}
	})

	t.Run("Try to add a user with a regular user active session", func(t *testing.T) {
		reset()

		response, err := getRequest(regularUserCookie, app, "/users/new", t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)

		response, err = postRequest(url.Values{}, regularUserCookie, app, "/users", t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
	})

	t.Run("Try to add a user with errors in form", func(t *testing.T) {
		reset()

		response, err := postRequest(url.Values{}, adminCookie, app, "/users", t)
		expectedErrorMessages := []string{
			"Name cannot be empty",
			"Username cannot be empty",
			"Incorrect email address",
			"Incorrect reading speed",
			"Confirm password cannot be empty",
			"Incorrect role",
		}
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		checkErrorMessages(response, t, expectedErrorMessages)
	})

	t.Run("Try to add a user with already registered email and username", func(t *testing.T) {
		reset()

		newUserData := url.Values{
			"name":             {"Test user"},
			"username":         {"regular"},
			"email":            {"regular@example.com"},
			"password":         {"test"},
			"confirm-password": {"test"},
			"role":             {fmt.Sprint(model.RoleRegular)},
			"words-per-minute": {"250"},
		}

		response, err := postRequest(newUserData, adminCookie, app, "/users", t)
		expectedErrorMessages := []string{
			"A user with this username already exists",
			"A user with this email address already exists",
		}
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		checkErrorMessages(response, t, expectedErrorMessages)
	})

	t.Run("Try to update a user without an active session", func(t *testing.T) {
		reset()

		response, err := getRequest(&http.Cookie{}, app, fmt.Sprintf("/users/%s", regularUser.Username), t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)

		response, err = putRequest(regularUserData, &http.Cookie{}, app, fmt.Sprintf("/users/%s", regularUser.Username), t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)
	})

	t.Run("Try to update a user using another, non admin user session", func(t *testing.T) {
		reset()

		adminUserData := regularUserData
		adminUserData.Set("id", adminUser.Uuid)

		response, err := getRequest(regularUserCookie, app, fmt.Sprintf("/users/%s", adminUser.Username), t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)

		response, err = putRequest(adminUserData, regularUserCookie, app, fmt.Sprintf("/users/%s", adminUser.Username), t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
	})

	t.Run("Try to update the user in session", func(t *testing.T) {
		reset()

		regularUserData.Set("name", "Updated regular user   ") // Extra spaces to test trimming
		regularUserData.Set("id", regularUser.Uuid)

		response, err := getRequest(regularUserCookie, app, fmt.Sprintf("/users/%s", regularUser.Username), t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		response, err = putRequest(regularUserData, regularUserCookie, app, fmt.Sprintf("/users/%s", regularUser.Username), t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		regularUser := model.User{}
		db.Where("email = ?", "regular@example.com").First(&regularUser)
		if expectedRegularUserName := "Updated regular user"; regularUser.Name != expectedRegularUserName {
			t.Errorf("User not updated, expecting name to be '%s' but got '%s'", expectedRegularUserName, regularUser.Name)
		}
	})

	t.Run("Try to update a user with an admin session", func(t *testing.T) {
		reset()

		regularUserData.Set("name", "Updated regular user by an admin")
		regularUserData.Set("id", regularUser.Uuid)

		response, err := putRequest(regularUserData, adminCookie, app, fmt.Sprintf("/users/%s", regularUser.Username), t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		response, err = putRequest(regularUserData, adminCookie, app, fmt.Sprintf("/users/%s", regularUser.Username), t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		regularUser := model.User{}
		db.Where("email = ?", "regular@example.com").First(&regularUser)
		if expectedRegularUserName := "Updated regular user by an admin"; regularUser.Name != expectedRegularUserName {
			t.Errorf("User not updated, expecting name to be '%s' but got '%s'", expectedRegularUserName, regularUser.Name)
		}
	})

	t.Run("Try to edit a non existing user with an admin session", func(t *testing.T) {
		reset()

		response, err := getRequest(adminCookie, app, fmt.Sprintf("/users/%s", "abcde"), t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		mustReturnStatus(response, fiber.StatusNotFound, t)
	})

	t.Run("Try to update a non existing user with an admin session", func(t *testing.T) {
		reset()

		regularUserData.Set("name", "Updated test user by an admin")

		response, err := putRequest(regularUserData, adminCookie, app, fmt.Sprintf("/users/%s", "abcde"), t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		mustReturnStatus(response, fiber.StatusNotFound, t)
	})

	t.Run("Try to delete a user without an active session", func(t *testing.T) {
		reset()

		response, err := deleteRequest(url.Values{}, &http.Cookie{}, app, fmt.Sprintf("/users/%s", regularUser.Username), t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)
	})

	t.Run("Try to delete a user with a regular user's session", func(t *testing.T) {
		reset()

		response, err := deleteRequest(url.Values{}, regularUserCookie, app, fmt.Sprintf("/users/%s", regularUser.Username), t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
	})

	t.Run("Try to delete a user with an admin session", func(t *testing.T) {
		reset()

		response, err := deleteRequest(url.Values{}, adminCookie, app, fmt.Sprintf("/users/%s", regularUser.Username), t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		var totalUsers int64
		db.Take(&[]model.User{}).Count(&totalUsers)

		if totalUsers != 1 {
			t.Errorf("Expected 1 users in the users list, got %d", totalUsers)
		}
	})

	t.Run("Try to delete the only existing admin user", func(t *testing.T) {
		reset()

		response, err := deleteRequest(url.Values{}, adminCookie, app, fmt.Sprintf("/users/%s", adminUser.Username), t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
	})

	t.Run("Try to delete a non existing user with an admin session", func(t *testing.T) {
		reset()

		response, err := deleteRequest(url.Values{}, adminCookie, app, "/users/wrong", t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		mustReturnStatus(response, fiber.StatusNotFound, t)
	})
}

func checkErrorMessages(response *http.Response, t *testing.T, expectedErrorMessages []string) {
	t.Helper()

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	errorMessages := []string{}
	doc.Find(".invalid-feedback").Each(func(i int, s *goquery.Selection) {
		errorMessages = append(errorMessages, strings.TrimSpace(s.Text()))
	})
	if !reflect.DeepEqual(expectedErrorMessages, errorMessages) {
		t.Errorf("Expected %v error messages, got %v", expectedErrorMessages, errorMessages)
	}
}

func mustRedirectToUsersList(response *http.Response, t *testing.T) {
	t.Helper()

	if response.StatusCode != http.StatusFound {
		t.Fatalf("Expected status %d, received %d", http.StatusFound, response.StatusCode)
	}
	url, err := response.Location()
	if err != nil {
		t.Fatal("No location header present")
	}
	if url.Path != "/users" {
		t.Errorf("Expected location %s, received %s", "/users", url.Path)
	}
}

func mustReturnStatus(response *http.Response, expectedStatus int, t *testing.T) {
	t.Helper()

	if response.StatusCode != expectedStatus {
		t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
	}
}

func login(app *fiber.App, email, password string, t *testing.T) (*http.Cookie, error) {
	t.Helper()

	data := url.Values{
		"email":    {email},
		"password": {password},
	}

	req, err := http.NewRequest(http.MethodPost, "/sessions", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	response, err := app.Test(req)
	if err != nil {
		return nil, err
	}

	if len(response.Cookies()) == 0 {
		return nil, fmt.Errorf("Cookie not set up")
	}
	return response.Cookies()[0], nil
}
