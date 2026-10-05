package webserver_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
)

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
