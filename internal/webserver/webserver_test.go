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
	"github.com/svera/coreander/internal/infrastructure"
	"github.com/svera/coreander/internal/metadata"
	"github.com/svera/coreander/internal/model"
	"github.com/svera/coreander/internal/webserver"
	"gorm.io/gorm"
)

func TestGET(t *testing.T) {
	var cases = []struct {
		name           string
		url            string
		expectedStatus int
	}{
		{"Redirect if the user tries to access to the root URL", "/", http.StatusFound},
		{"Page loads successfully if the user tries to access the spanish version", "/es", http.StatusOK},
		{"Page loads successfully if the user tries to access the english version", "/en", http.StatusOK},
		{"Server returns not found if the user tries to access a non-existent URL", "/xx", http.StatusNotFound},
	}

	db := infrastructure.Connect("file::memory:?cache=shared", 250)
	app := bootstrapApp(db, &infrastructure.NoEmail{})

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, tcase.url, nil)

			body, err := app.Test(req)
			if err != nil {
				t.Errorf("Unexpected error: %v", err.Error())
			}
			if body.StatusCode != tcase.expectedStatus {
				t.Errorf("Wrong status code received, expected %d, got %d", tcase.expectedStatus, body.StatusCode)
			}
		})
	}
}

func TestUserManagement(t *testing.T) {
	db := infrastructure.Connect("file::memory:?cache=shared", 250)
	app := bootstrapApp(db, &infrastructure.NoEmail{})

	data := url.Values{
		"name":             {"Test user"},
		"email":            {"test@example.com"},
		"password":         {"test"},
		"confirm-password": {"test"},
		"role":             {"1"},
		"words-per-minute": {"250"},
	}

	adminCookie, err := login(app, "admin@example.com", "admin")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	t.Run("Try to add a user without an active session", func(t *testing.T) {
		response, err := newUser(&http.Cookie{}, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustRedirectToLogin(response, t)

		response, err = addUser(data, &http.Cookie{}, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustRedirectToLogin(response, t)
	})

	t.Run("Try to add a user with an admin active session", func(t *testing.T) {
		response, err := newUser(adminCookie, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		response, err = addUser(data, adminCookie, app)
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

	t.Run("Try to add a user with errors in form using an admin active session", func(t *testing.T) {
		response, err := addUser(url.Values{}, adminCookie, app)
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

	t.Run("Try to add a user with a regular user active session", func(t *testing.T) {
		cookie, err := login(app, "test@example.com", "test")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		response, err := addUser(data, cookie, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
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
		response, err := editUser(testUser.Uuid, &http.Cookie{}, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustRedirectToLogin(response, t)

		response, err = updateUser(testUser.Uuid, data, &http.Cookie{}, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustRedirectToLogin(response, t)
	})

	t.Run("Try to update a user using another, non admin user session", func(t *testing.T) {
		response, err := editUser(adminUser.Uuid, testUserCookie, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)

		response, err = updateUser(adminUser.Uuid, data, testUserCookie, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
	})

	t.Run("Try to update the user in session", func(t *testing.T) {
		data.Set("name", "Updated test user")

		response, err := editUser(testUser.Uuid, testUserCookie, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		response, err = updateUser(testUser.Uuid, data, testUserCookie, app)
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

		response, err := updateUser(testUser.Uuid, data, adminCookie, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		response, err = updateUser(testUser.Uuid, data, adminCookie, app)
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
		response, err := editUser("abcde", adminCookie, app)
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

		response, err := updateUser("abcde", data, adminCookie, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		mustReturnStatus(response, fiber.StatusNotFound, t)
	})

	data = url.Values{
		"uuid": {testUser.Uuid},
	}

	t.Run("Try to delete a user without an active session", func(t *testing.T) {
		response, err := deleteUser(data, &http.Cookie{}, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustRedirectToLogin(response, t)
	})

	t.Run("Try to delete a user with a regular user's session", func(t *testing.T) {
		data.Set("name", "Updated test user")

		response, err := deleteUser(data, testUserCookie, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
	})

	t.Run("Try to delete a user with an admin session", func(t *testing.T) {
		response, err := deleteUser(data, adminCookie, app)
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
			"uuid": {adminUser.Uuid},
		}
		response, err := deleteUser(data, adminCookie, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
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

func mustRedirectToLogin(response *http.Response, t *testing.T) {
	if response.StatusCode != http.StatusFound {
		t.Errorf("Expected status %d, received %d", http.StatusFound, response.StatusCode)
		return
	}
	url, err := response.Location()
	if err != nil {
		t.Error("No location header present")
		return
	}
	if url.Path != "/en/login" {
		t.Errorf("Expected location %s, received %s", "/en/login", url.Path)
	}
}

func mustReturnStatus(response *http.Response, expectedStatus int, t *testing.T) {
	if response.StatusCode != expectedStatus {
		t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
	}
}

func bootstrapApp(db *gorm.DB, sender webserver.Sender) *fiber.App {
	metadataReadersMock := map[string]metadata.Reader{
		"epub": metadata.NewReaderMock(),
	}

	webserverConfig := webserver.Config{
		CoverMaxWidth: 300,
	}
	return webserver.New(webserver.NewReaderMock(), webserverConfig, metadataReadersMock, sender, db)
}

func newUser(cookie *http.Cookie, app *fiber.App) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, "/en/users/new", nil)
	if err != nil {
		return nil, err
	}
	req.AddCookie(cookie)

	return app.Test(req)
}

func addUser(data url.Values, cookie *http.Cookie, app *fiber.App) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, "/en/users/new", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)

	return app.Test(req)
}

func editUser(uuid string, cookie *http.Cookie, app *fiber.App) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/en/users/%s/edit", uuid), nil)
	if err != nil {
		return nil, err
	}
	req.AddCookie(cookie)

	return app.Test(req)
}

func updateUser(uuid string, data url.Values, cookie *http.Cookie, app *fiber.App) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("/en/users/%s/edit", uuid), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)

	return app.Test(req)
}

func deleteUser(data url.Values, cookie *http.Cookie, app *fiber.App) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, "/en/users/delete", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)

	return app.Test(req)
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

	return response.Cookies()[0], nil
}
