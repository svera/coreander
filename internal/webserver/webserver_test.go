package webserver_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

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
		{"Page loads succesfully if the user tries to access the spanish version", "/es", http.StatusOK},
		{"Page loads succesfully if the user tries to access the english version", "/en", http.StatusOK},
		{"Server returns not found if the user tries to access a non-existent URL", "/xx", http.StatusNotFound},
	}

	db := infrastructure.Connect("file::memory:?cache=shared")
	app := bootstrapApp(db)

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

func TestAddNewUser(t *testing.T) {
	db := infrastructure.Connect("file::memory:?cache=shared")
	app := bootstrapApp(db)

	data := url.Values{
		"name":             {"Test user"},
		"email":            {"test@example.com"},
		"password":         {"test"},
		"confirm-password": {"test"},
		"role":             {"1"},
	}

	adminCookie, err := login(app, "admin@example.com", "admin")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	t.Run("Try to add a user without an active session", func(t *testing.T) {
		response, err := addUser(data, &http.Cookie{}, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		shouldRedirectToLogin(response, t)
	})

	t.Run("Try to add a user with an admin active session", func(t *testing.T) {
		response, err := addUser(data, adminCookie, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		shouldRedirectToUsersList(response, t)

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
		response, err := addUser(data, cookie, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		} else if response.StatusCode != http.StatusForbidden {
			t.Errorf("Expected status %d, received %d", http.StatusForbidden, response.StatusCode)
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
		response, err := updateUser(testUser.Uuid, data, &http.Cookie{}, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		shouldRedirectToLogin(response, t)
	})

	t.Run("Try to update a user using another, non admin user session", func(t *testing.T) {
		response, err := updateUser(adminUser.Uuid, data, testUserCookie, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		shouldReturnForbidden(response, t)
	})

	t.Run("Try to update the user in session", func(t *testing.T) {
		data.Set("name", "Updated test user")

		response, err := updateUser(testUser.Uuid, data, testUserCookie, app)
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
		testUser := model.User{}
		db.Where("email = ?", "test@example.com").First(&testUser)
		if testUser.Name != "Updated test user by an admin" {
			t.Errorf("User not updated, expecting name to be '%s' but got '%s'", "Updated test user by an admin", testUser.Name)
		}
	})

	data = url.Values{
		"uuid": {testUser.Uuid},
	}
	t.Run("Try to delete a user without an active session", func(t *testing.T) {
		response, err := deleteUser(testUser.Uuid, data, &http.Cookie{}, app)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		shouldRedirectToLogin(response, t)
	})

}

func shouldRedirectToUsersList(response *http.Response, t *testing.T) {
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

func shouldRedirectToLogin(response *http.Response, t *testing.T) {
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

func shouldReturnForbidden(response *http.Response, t *testing.T) {
	if response.StatusCode != http.StatusForbidden {
		t.Errorf("Expected status %d, received %d", http.StatusForbidden, response.StatusCode)
	}
}

func bootstrapApp(db *gorm.DB) *fiber.App {
	metadataReadersMock := map[string]metadata.Reader{
		"epub": metadata.NewReaderMock(),
	}

	webserverConfig := webserver.Config{
		CoverMaxWidth: 300,
	}
	return webserver.New(webserver.NewReaderMock(), webserverConfig, metadataReadersMock, &infrastructure.NoEmail{}, db)
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

func updateUser(uuid string, data url.Values, cookie *http.Cookie, app *fiber.App) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("/en/users/%s/edit", uuid), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)

	return app.Test(req)
}

func deleteUser(uuid string, data url.Values, cookie *http.Cookie, app *fiber.App) (*http.Response, error) {
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
