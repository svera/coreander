package webserver_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/versioncheck"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
)

func TestNavVersionNoticeForAdminsOnly(t *testing.T) {
	checker := versioncheck.NewWithFetcher("v1.0.0", func() (string, error) {
		return "v9.9.9", nil
	})
	checker.Refresh()

	config := defaultTestConfig()
	config.Version = "v1.0.0"
	config.VersionChecker = checker

	db := infrastructure.Connect(":memory:", 250)
	app := bootstrapApp(db, &infrastructure.NoEmail{}, afero.NewMemMapFs(), config)

	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatalf("login admin: %v", err)
	}

	t.Run("admin sees update notice in nav", func(t *testing.T) {
		response, err := getRequest(adminCookie, app, "/", t)
		if err != nil {
			t.Fatalf("GET /: %v", err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.StatusCode)
		}

		doc, err := goquery.NewDocumentFromReader(response.Body)
		if err != nil {
			t.Fatalf("parse HTML: %v", err)
		}
		nav := doc.Find("nav").Text()
		if !strings.Contains(nav, "New version available") {
			t.Fatalf("nav should mention new version, got: %q", nav)
		}
		if doc.Find(`nav a[href="https://github.com/svera/coreander/releases/latest"]`).Length() == 0 {
			t.Fatal("nav should contain download link for latest release")
		}
		if strings.Contains(doc.Find("footer").Text(), "New version available") {
			t.Fatal("footer should not show update notice")
		}
	})

	t.Run("unauthenticated user does not see update notice", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Accept-Language", "en")
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET /: %v", err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.StatusCode)
		}

		doc, err := goquery.NewDocumentFromReader(response.Body)
		if err != nil {
			t.Fatalf("parse HTML: %v", err)
		}
		nav := doc.Find("nav").Text()
		if strings.Contains(nav, "New version available") {
			t.Fatalf("nav should not show update notice, got: %q", nav)
		}
	})
}
