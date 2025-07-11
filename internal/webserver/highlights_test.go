package webserver_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"gorm.io/gorm"
)

func TestHighlights(t *testing.T) {
	var (
		db          *gorm.DB
		app         *fiber.App
		adminCookie *http.Cookie
		adminUser   model.User
	)

	reset := func() {
		var err error

		db = infrastructure.Connect(":memory:", 250)
		appFS := loadFilesInMemoryFs([]string{"fixtures/library/metadata.epub"})
		app = bootstrapApp(db, &infrastructure.NoEmail{}, appFS, webserver.Config{})
		adminCookie, err = login(app, "admin@example.com", "admin", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		adminUser = model.User{}
		db.Where("email = ?", "admin@example.com").First(&adminUser)

		regularUserData := url.Values{
			"name":             {"Test user"},
			"username":         {"test"},
			"email":            {"test@example.com"},
			"password":         {"test"},
			"confirm-password": {"test"},
			"role":             {fmt.Sprint(model.RoleRegular)},
			"words-per-minute": {"250"},
		}

		response, err := postRequest(regularUserData, adminCookie, app, "/users", t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
	}

	reset()

	t.Run("Try to highlight a document without an active session", func(t *testing.T) {
		t.Cleanup(reset)

		response, err := highlight(&http.Cookie{}, app, "john-doe-test-epub", fiber.MethodPost, t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)
	})

	t.Run("Try to highlight and dehighlight a document with an active session", func(t *testing.T) {
		t.Cleanup(reset)

		response, err := highlight(adminCookie, app, "john-doe-test-epub", fiber.MethodPost, t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		assertHighlights(app, t, adminCookie, 1)

		response, err = highlight(adminCookie, app, "john-doe-test-epub", fiber.MethodDelete, t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		assertHighlights(app, t, adminCookie, 0)
	})

	t.Run("Deleting a document also removes it from the highlights of all users", func(t *testing.T) {
		t.Cleanup(reset)

		regularUser := model.User{}
		db.Where("email = ?", "test@example.com").First(&regularUser)

		regularUserCookie, err := login(app, "test@example.com", "test", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		response, err := highlight(regularUserCookie, app, "john-doe-test-epub", fiber.MethodPost, t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		assertHighlights(app, t, regularUserCookie, 1)

		adminCookie, err = login(app, "admin@example.com", "admin", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		_, err = deleteRequest(url.Values{}, adminCookie, app, "/documents/john-doe-test-epub", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		var total int64
		db.Table("highlights").Where("user_id = ?", regularUser.ID).Count(&total)
		if total != 0 {
			t.Errorf("Expected no highlights in DB for user, got %d", total)
		}
		assertHighlights(app, t, adminCookie, 0)
	})

	t.Run("Deleting a user also remove his/her highlights", func(t *testing.T) {
		t.Cleanup(reset)

		regularUser := model.User{}
		db.Where("email = ?", "test@example.com").First(&regularUser)

		regularUserCookie, err := login(app, "test@example.com", "test", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		response, err := highlight(regularUserCookie, app, "john-doe-test-epub", fiber.MethodPost, t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		assertHighlights(app, t, regularUserCookie, 1)

		adminCookie, err = login(app, "admin@example.com", "admin", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if _, err = deleteRequest(url.Values{}, adminCookie, app, fmt.Sprintf("/users/%s", regularUser.Username), t); err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		var total int64
		db.Table("highlights").Where("user_id = ?", regularUser.ID).Count(&total)
		if total != 0 {
			t.Errorf("Expected no highlights in DB for deleted user, got %d", total)
		}
	})
}

func highlight(cookie *http.Cookie, app *fiber.App, slug string, method string, t *testing.T) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest(method, fmt.Sprintf("/highlights/%s", slug), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept-Language", "en")
	req.AddCookie(cookie)

	return app.Test(req)
}

func assertHighlights(app *fiber.App, t *testing.T, cookie *http.Cookie, expectedResults int) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, "/highlights", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	req.Header.Set("Accept-Language", "en")
	req.AddCookie(cookie)
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if expectedStatus := http.StatusOK; response.StatusCode != expectedStatus {
		t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if actualResults := doc.Find("#list .list-group-item").Length(); actualResults != expectedResults {
		t.Errorf("Expected %d results, got %d", expectedResults, actualResults)
	}
}

func assertNoHighlights(app *fiber.App, t *testing.T, cookie *http.Cookie) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, "/highlights", nil)
	req.Header.Set("Accept-Language", "en")
	req.AddCookie(cookie)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if expectedStatus := http.StatusNotFound; response.StatusCode != expectedStatus {
		t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
	}
}
