package webserver_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
	"github.com/svera/coreander/v3/internal/webserver/model"
	"gorm.io/gorm"
)

var (
	db          *gorm.DB
	app         *fiber.App
	adminCookie *http.Cookie
	data        url.Values
	adminUser   model.User
)

func setup(t *testing.T) {
	var err error

	db = infrastructure.Connect("file::memory:", 250)
	appFS := loadFilesInMemoryFs([]string{"fixtures/library/metadata.epub"})
	app = bootstrapApp(db, &infrastructure.NoEmail{}, appFS)
	adminCookie, err = login(app, "admin@example.com", "admin")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	data = url.Values{
		"slug": {"john-doe-test-epub"},
	}
	adminUser = model.User{}
	db.Where("email = ?", "admin@example.com").First(&adminUser)

	regularUserData := url.Values{
		"name":             {"Test user"},
		"email":            {"test@example.com"},
		"password":         {"test"},
		"confirm-password": {"test"},
		"role":             {fmt.Sprint(model.RoleRegular)},
		"words-per-minute": {"250"},
	}

	response, err := postRequest(regularUserData, adminCookie, app, "/en/users/new")
	if response == nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
}

func TestHighlights(t *testing.T) {
	t.Run("Try to highlight a document without an active session", func(t *testing.T) {
		setup(t)
		response, err := highlight(&http.Cookie{}, app, strings.NewReader(data.Encode()), fiber.MethodPost)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)
	})

	t.Run("Try to highlight and dehighlight a document with an active session", func(t *testing.T) {
		setup(t)
		response, err := highlight(adminCookie, app, strings.NewReader(data.Encode()), fiber.MethodPost)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		assertHighlights(app, t, adminCookie, adminUser.Uuid, 1)

		response, err = highlight(adminCookie, app, strings.NewReader(data.Encode()), fiber.MethodDelete)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		assertHighlights(app, t, adminCookie, adminUser.Uuid, 0)
	})

	t.Run("Deleting a document also removes it from the highlights of all users", func(t *testing.T) {
		setup(t)
		regularUser := model.User{}
		db.Where("email = ?", "test@example.com").First(&regularUser)

		regularUserCookie, err := login(app, "test@example.com", "test")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		response, err := highlight(regularUserCookie, app, strings.NewReader(data.Encode()), fiber.MethodPost)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		assertHighlights(app, t, regularUserCookie, regularUser.Uuid, 1)

		adminCookie, err = login(app, "admin@example.com", "admin")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		_, err = postRequest(data, adminCookie, app, "/delete")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		var total int64
		db.Table("highlights").Where("user_id = ?", regularUser.ID).Count(&total)
		if total != 0 {
			t.Errorf("Expected no highlights in DB for user, got %d", total)
		}
		assertHighlights(app, t, adminCookie, regularUser.Uuid, 0)
	})

	t.Run("Deleting a user also remove his/her highlights", func(t *testing.T) {
		setup(t)
		regularUser := model.User{}
		db.Where("email = ?", "test@example.com").First(&regularUser)

		regularUserCookie, err := login(app, "test@example.com", "test")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		response, err := highlight(regularUserCookie, app, strings.NewReader(data.Encode()), fiber.MethodPost)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusOK, t)

		assertHighlights(app, t, regularUserCookie, regularUser.Uuid, 1)

		adminCookie, err = login(app, "admin@example.com", "admin")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		data := url.Values{
			"uuid": {regularUser.Uuid},
		}

		_, err = postRequest(data, adminCookie, app, "/en/users/delete")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		var total int64
		db.Table("highlights").Where("user_id = ?", regularUser.ID).Count(&total)
		if total != 0 {
			t.Errorf("Expected no highlights in DB for deleted user, got %d", total)
		}
		assertNoHighlights(app, t, adminCookie, regularUser.Uuid)
	})
}

func highlight(cookie *http.Cookie, app *fiber.App, reader *strings.Reader, method string) (*http.Response, error) {
	req, err := http.NewRequest(method, "/highlights", reader)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)

	return app.Test(req)
}

func assertHighlights(app *fiber.App, t *testing.T, cookie *http.Cookie, uuid string, expectedResults int) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/en/highlights/%s", uuid), nil)
	req.AddCookie(cookie)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
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

	if actualResults := doc.Find(".list-group-item").Length(); actualResults != expectedResults {
		t.Errorf("Expected %d results, got %d", expectedResults, actualResults)
	}
}

func assertNoHighlights(app *fiber.App, t *testing.T, cookie *http.Cookie, uuid string) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/en/highlights/%s", uuid), nil)
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
