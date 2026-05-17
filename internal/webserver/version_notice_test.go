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

func TestFooterVersionNoticeForAdminsOnly(t *testing.T) {
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

	t.Run("admin sees update notice in footer", func(t *testing.T) {
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
		footer := doc.Find("footer").Text()
		if !strings.Contains(footer, "v9.9.9") {
			t.Fatalf("footer should mention latest version, got: %q", footer)
		}
		if doc.Find(`footer a[href="https://github.com/svera/coreander/releases/latest"]`).Length() == 0 {
			t.Fatal("footer should contain download link for latest release")
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
		footer := doc.Find("footer").Text()
		if strings.Contains(footer, "A new version") {
			t.Fatalf("footer should not show update notice, got: %q", footer)
		}
	})
}
