package webserver_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/metadata"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

func TestDocumentPageTitles(t *testing.T) {
	for _, tc := range []struct {
		name    string
		authors []string
		title   string
		author  string
	}{
		{name: "no authors", title: "TheArcade_PDF"},
		{name: "empty author", authors: []string{""}, title: "TheArcade_PDF"},
		{name: "multiple empty authors", authors: []string{"", ""}, title: "TheArcade_PDF"},
		{name: "whitespace authors", authors: []string{" ", "\t"}, title: "TheArcade_PDF"},
		{name: "one author", authors: []string{"Jane Doe"}, title: "Jane Doe - TheArcade_PDF", author: "Jane Doe"},
		{name: "multiple authors", authors: []string{"Jane Doe", "John Doe"}, title: "Jane Doe, John Doe - TheArcade_PDF", author: "Jane Doe, John Doe"},
		{name: "mixed authors", authors: []string{"", " Jane Doe ", "\t"}, title: "Jane Doe - TheArcade_PDF", author: "Jane Doe"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := infrastructure.Connect(":memory:", 250)
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			appFs := afero.NewMemMapFs()
			const path = testLibraryDir + "/TheArcade_PDF.pdf"
			if err := afero.WriteFile(appFs, path, []byte("test document"), 0644); err != nil {
				t.Fatal(err)
			}
			reader := catalogReader{byPath: map[string]metadata.Metadata{
				path: {Title: "TheArcade_PDF", Authors: tc.authors, Format: "PDF"},
			}}
			app := bootstrapApp(db, &infrastructure.NoEmail{}, appFs, webserver.Config{},
				map[string]metadata.Reader{".pdf": reader})
			t.Cleanup(func() { _ = app.Shutdown() })

			response, err := app.Test(mustGetRequest(t, "/search?type=documents"))
			if err != nil {
				t.Fatal(err)
			}
			home, err := goquery.NewDocumentFromReader(response.Body)
			_ = response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			detailPath, ok := home.Find("a[href^='/documents/']").First().Attr("href")
			if !ok {
				t.Fatal("document detail link not found")
			}
			for _, page := range []struct {
				path  string
				title string
			}{
				{path: detailPath, title: tc.title + " | Coreander"},
				{path: detailPath + "/read", title: tc.title},
			} {
				resp, err := app.Test(mustGetRequest(t, page.path))
				if err != nil {
					t.Fatal(err)
				}
				doc, err := goquery.NewDocumentFromReader(resp.Body)
				_ = resp.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("%s status = %d", page.path, resp.StatusCode)
				}
				if got := doc.Find("title").Text(); got != page.title {
					t.Errorf("%s title = %q, want %q", page.path, got, page.title)
				}
				if page.path == detailPath+"/read" {
					if got, ok := doc.Find("meta[name='author']").Attr("content"); !ok || got != tc.author {
						t.Errorf("%s author metadata = %q (present: %v), want %q", page.path, got, ok, tc.author)
					}
				}
			}
		})
	}
}

func mustGetRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestDocumentDetail(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	app := bootstrapApp(db, smtpMock, afero.NewOsFs(), webserver.Config{})

	var cases = []struct {
		url            string
		expectedStatus int
	}{
		{"/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha", http.StatusOK},
		{"/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha--2", http.StatusOK},
		{"/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha--3", http.StatusOK},
		{"/documents/john-doe-non-existing-document", http.StatusNotFound},
	}

	for _, tcase := range cases {
		t.Run(tcase.url, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tcase.url, nil)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
			response, err := app.Test(req)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
			if response.StatusCode != tcase.expectedStatus {
				t.Errorf("Expected status %d, received %d", tcase.expectedStatus, response.StatusCode)
			}
		})
	}
}

func TestReaderDownloadUsesPageOrigin(t *testing.T) {
	for _, tc := range []struct {
		name string
		page string
		fqdn string
	}{
		{"LAN IP without configured port", "http://192.168.1.139:3000", "192.168.1.139"},
		{"LAN access with default FQDN", "http://192.168.1.139:4000", "localhost:3000"},
		{"HTTPS proxy with different FQDN", "https://reader.example.com", "internal.example.com:3000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := infrastructure.Connect(":memory:", 250)
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			cfg := defaultTestConfig()
			cfg.FQDN = tc.fqdn
			app := bootstrapApp(db, &infrastructure.NoEmail{},
				loadFilesInMemoryFs([]string{"testdata/library/metadata.epub"}), cfg)
			adminCookie, err := login(app, "admin@example.com", "admin", t)
			if err != nil {
				t.Fatal(err)
			}
			const downloadPath = "/documents/john-doe-test-epub/download"
			for _, cookie := range []*http.Cookie{nil, adminCookie} {
				req, err := http.NewRequest(http.MethodGet, tc.page+"/documents/john-doe-test-epub/read", nil)
				if err != nil {
					t.Fatal(err)
				}
				if cookie != nil {
					req.AddCookie(cookie)
				}
				resp, err := app.Test(req)
				if err != nil {
					t.Fatal(err)
				}
				page, err := goquery.NewDocumentFromReader(resp.Body)
				_ = resp.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("reader status = %d", resp.StatusCode)
				}
				if !page.Find("#reader-toast").Is("div[hidden][role='alert']") {
					t.Fatal("reader notifications must use an initially hidden non-dialog banner")
				}
				value, ok := page.Find("#url").Attr("value")
				if !ok || value != downloadPath {
					t.Fatalf("download URL = %q, want %q", value, downloadPath)
				}
				reference, err := url.Parse(value)
				if err != nil {
					t.Fatal(err)
				}
				downloadURL := req.URL.ResolveReference(reference)
				if downloadURL.Scheme != req.URL.Scheme || downloadURL.Host != req.URL.Host {
					t.Fatalf("download origin differs from reader: %s", downloadURL)
				}
				downloadReq, err := http.NewRequest(http.MethodGet, downloadURL.String(), nil)
				if err != nil {
					t.Fatal(err)
				}
				if cookie != nil {
					downloadReq.AddCookie(cookie)
				}
				downloadResp, err := app.Test(downloadReq)
				if err != nil {
					t.Fatal(err)
				}
				_ = downloadResp.Body.Close()
				if downloadResp.StatusCode != http.StatusOK {
					t.Fatalf("download status = %d", downloadResp.StatusCode)
				}
			}
		})
	}
}

func TestDocumentReadAndDeleteDocumentAfterwards(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	appFS := loadFilesInMemoryFs([]string{"testdata/library/quijote.epub"})
	smtpMock := &infrastructure.SMTPMock{}
	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	var cases = []struct {
		url            string
		expectedStatus int
	}{
		{"/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha", http.StatusOK},
		{"/documents/john-doe-non-existing-document", http.StatusNotFound},
	}

	for _, tcase := range cases {
		t.Run(tcase.url, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tcase.url+"/read", nil)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
			req.AddCookie(adminCookie)
			response, err := app.Test(req)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
			if response.StatusCode != tcase.expectedStatus {
				t.Errorf("Expected status %d, received %d", tcase.expectedStatus, response.StatusCode)
			}

			if tcase.expectedStatus != http.StatusOK {
				return
			}

			if !isProgressSectionShownInHome(t, app, adminCookie) {
				t.Errorf("Expected to have a resume reading section in home")
			}

			if response, err = deleteRequest(url.Values{}, adminCookie, app, tcase.url, t); err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}

			if response.StatusCode != http.StatusOK {
				t.Errorf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
			}

			if isProgressSectionShownInHome(t, app, adminCookie) {
				t.Errorf("Expected to not have a resume reading section in home after removing the document")
			}

			var total int64
			if db.Table("readings").Where("slug = ?", "miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha").Count(&total); total != 0 {
				t.Errorf("Expected no entries in DB readings table for document, got %d", total)
			}
		})
	}
}

func TestDocumentReadAndDeleteUserAfterwards(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	appFS := loadFilesInMemoryFs([]string{"testdata/library/quijote.epub"})
	smtpMock := &infrastructure.SMTPMock{}
	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	addRegularUser(t, app, adminCookie)

	regularUser := model.User{}
	db.Where("email = ?", "regular@example.com").First(&regularUser)

	regularUserCookie, err := login(app, "regular@example.com", "regular", t)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	req, err := http.NewRequest(http.MethodGet, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/read", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	req.AddCookie(regularUserCookie)
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if response.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	if response, err = deleteRequest(url.Values{}, adminCookie, app, "/users/regular", t); err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	if response.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	var total int64
	if db.Table("readings").Where("user_id = ?", regularUser.ID).Count(&total); total != 0 {
		t.Errorf("Expected no entries in DB readings table for user, got %d", total)
	}
}

func isProgressSectionShownInHome(t *testing.T, app *fiber.App, cookie *http.Cookie) bool {
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
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

	return doc.Find("#in-progress-docs").Length() == 1
}

func addRegularUser(t *testing.T, app *fiber.App, adminCookie *http.Cookie) {
	regularUserData := url.Values{
		"name":             {"Regular user"},
		"username":         {"regular"},
		"email":            {"regular@example.com"},
		"password":         {"regular"},
		"confirm-password": {"regular"},
		"role":             {fmt.Sprint(model.RoleRegular)},
		"words-per-minute": {"250"},
	}

	if _, err := postRequest(regularUserData, adminCookie, app, "/users", t); err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
}
