package webserver_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/infrastructure"
	"github.com/svera/coreander/v4/internal/model"
)

func TestHighlights(t *testing.T) {
	db := infrastructure.Connect("file::memory:", 250)
	appFS := loadFilesInMemoryFs([]string{"fixtures/metadata.epub"})
	app := bootstrapApp(db, &infrastructure.NoEmail{}, appFS)
	data := url.Values{
		"slug": {"john-doe-test-epub"},
	}
	adminUser := model.User{}
	db.Where("email = ?", "admin@example.com").First(&adminUser)

	adminCookie, err := login(app, "admin@example.com", "admin")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	t.Run("Try to highlight a document without an active session", func(t *testing.T) {
		response, err := highlight(&http.Cookie{}, app, strings.NewReader(data.Encode()), fiber.MethodPost)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)
	})

	t.Run("Try to highlight and dehighlight a document with an active session", func(t *testing.T) {
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
