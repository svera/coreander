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
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
	"github.com/svera/coreander/v3/internal/webserver/model"
)

func TestUserManagement(t *testing.T) {
	db := infrastructure.Connect("file::memory:", 250)
	app := bootstrapApp(db, &infrastructure.NoEmail{}, afero.NewMemMapFs())

	data := url.Values{
		"name":             {"Test user"},
		"email":            {"test@example.com"},
		"password":         {"test"},
		"confirm-password": {"test"},
		"role":             {fmt.Sprint(model.RoleRegular)},
		"words-per-minute": {"250"},
	}

	adminCookie, err := login(app, "admin@example.com", "admin")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	t.Run("Try to add a user without an active session", func(t *testing.T) {
		response, err := getRequest(&http.Cookie{}, app, "/en/users/new")
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)

		response, err = postRequest(data, &http.Cookie{}, app, "/en/users/new")
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)
	})

	t.Run("Try to add a user with an admin active session", func(t *testing.T) {
		response, err := getRequest(adminCookie, app, "/en/users/new")
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		response, err = postRequest(data, adminCookie, app, "/en/users/new")
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		mustRedirectToUsersList(response, t)

		var totalUsers int64
		db.Take(&[]model.User{}).Count(&totalUsers)

		if totalUsers != 2 {
			t.Errorf("Expected 2 users in the users list, got %d", totalUsers)
		}
	})

	t.Run("Try to add a user with a regular user active session", func(t *testing.T) {
		cookie, err := login(app, "test@example.com", "test")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		response, err := getRequest(cookie, app, "/en/users/new")
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)

		response, err = postRequest(data, cookie, app, "/en/users/new")
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
	})

	t.Run("Try to add a user with errors in form using an admin active session", func(t *testing.T) {
		response, err := postRequest(url.Values{}, adminCookie, app, "/en/users/new")
		expectedErrorMessages := []string{
			"Name cannot be empty",
			"Incorrect email address",
			"Incorrect reading speed",
			"Confirm password cannot be empty",
			"Incorrect role",
		}
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

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
	})

	testUser := model.User{}
	db.Where("email = ?", "test@example.com").First(&testUser)

	adminUser := model.User{}
	db.Where("email = ?", "admin@example.com").First(&adminUser)

	testUserCookie, err := login(app, "test@example.com", "test")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	t.Run("Try to update a user without an active session", func(t *testing.T) {
		response, err := getRequest(&http.Cookie{}, app, fmt.Sprintf("/en/users/%s/edit", testUser.Uuid))
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)

		response, err = postRequest(data, &http.Cookie{}, app, fmt.Sprintf("/en/users/%s/edit", testUser.Uuid))
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)
	})

	t.Run("Try to update a user using another, non admin user session", func(t *testing.T) {
		response, err := getRequest(testUserCookie, app, fmt.Sprintf("/en/users/%s/edit", adminUser.Uuid))
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)

		response, err = postRequest(data, testUserCookie, app, fmt.Sprintf("/en/users/%s/edit", adminUser.Uuid))
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
	})

	t.Run("Try to update the user in session", func(t *testing.T) {
		data.Set("name", "Updated test user")

		response, err := getRequest(testUserCookie, app, fmt.Sprintf("/en/users/%s/edit", testUser.Uuid))
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		response, err = postRequest(data, testUserCookie, app, fmt.Sprintf("/en/users/%s/edit", testUser.Uuid))
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		testUser := model.User{}
		db.Where("email = ?", "test@example.com").First(&testUser)
		if testUser.Name != "Updated test user" {
			t.Errorf("User not updated, expecting name to be '%s' but got '%s'", "Updated test user", testUser.Name)
		}
	})

	t.Run("Try to update a user with an admin session", func(t *testing.T) {
		data.Set("name", "Updated test user by an admin")

		response, err := postRequest(data, adminCookie, app, fmt.Sprintf("/en/users/%s/edit", testUser.Uuid))
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		response, err = postRequest(data, adminCookie, app, fmt.Sprintf("/en/users/%s/edit", testUser.Uuid))
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		testUser := model.User{}
		db.Where("email = ?", "test@example.com").First(&testUser)
		if testUser.Name != "Updated test user by an admin" {
			t.Errorf("User not updated, expecting name to be '%s' but got '%s'", "Updated test user by an admin", testUser.Name)
		}
	})

	t.Run("Try to edit a non existing user with an admin session", func(t *testing.T) {
		response, err := getRequest(adminCookie, app, fmt.Sprintf("/en/users/%s/edit", "abcde"))
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		mustReturnStatus(response, fiber.StatusNotFound, t)
	})

	data = url.Values{
		"uuid": {testUser.Uuid},
	}

	t.Run("Try to update a non existing user with an admin session", func(t *testing.T) {
		data.Set("name", "Updated test user by an admin")

		response, err := postRequest(data, adminCookie, app, fmt.Sprintf("/en/users/%s/edit", "abcde"))
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		mustReturnStatus(response, fiber.StatusNotFound, t)
	})

	data = url.Values{
		"id": {testUser.Uuid},
	}

	t.Run("Try to delete a user without an active session", func(t *testing.T) {
		response, err := deleteRequest(data, &http.Cookie{}, app, "/users")
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)
	})

	t.Run("Try to delete a user with a regular user's session", func(t *testing.T) {
		data.Set("name", "Updated test user")

		response, err := deleteRequest(data, testUserCookie, app, "/users")
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
	})

	t.Run("Try to delete a user with an admin session", func(t *testing.T) {
		response, err := deleteRequest(data, adminCookie, app, "/users")
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
		data = url.Values{
			"id": {adminUser.Uuid},
		}
		response, err := deleteRequest(data, adminCookie, app, "/users")
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
	})

	t.Run("Try to delete a non existing user with an admin session", func(t *testing.T) {
		data = url.Values{
			"id": {"abcde"},
		}

		response, err := deleteRequest(data, adminCookie, app, "/users")
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		mustReturnStatus(response, fiber.StatusNotFound, t)
	})
}

func mustRedirectToUsersList(response *http.Response, t *testing.T) {
	if response.StatusCode != http.StatusFound {
		t.Fatalf("Expected status %d, received %d", http.StatusFound, response.StatusCode)
	}
	url, err := response.Location()
	if err != nil {
		t.Fatal("No location header present")
	}
	if url.Path != "/en/users" {
		t.Errorf("Expected location %s, received %s", "/en/users", url.Path)
	}
}

func mustReturnStatus(response *http.Response, expectedStatus int, t *testing.T) {
	if response.StatusCode != expectedStatus {
		t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
	}
}

func login(app *fiber.App, email, password string) (*http.Cookie, error) {
	data := url.Values{
		"email":    {email},
		"password": {password},
	}

	req, err := http.NewRequest(http.MethodPost, "/en/login", strings.NewReader(data.Encode()))
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
