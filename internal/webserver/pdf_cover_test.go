package webserver_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
)

func TestPDFCoverMarkup(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	cfg := defaultTestConfig()
	cfg.CoverMaxWidth = 600
	app := bootstrapApp(db, &infrastructure.NoEmail{},
		loadFilesInMemoryFs([]string{"testdata/library/metadata.pdf", "testdata/library/metadata.epub"}), cfg)
	for _, path := range []string{"/search?keywords=Test", "/documents/john-doe-test-pdf", "/documents/john-doe-test-epub"} {
		t.Run(path, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, path, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			page, err := goquery.NewDocumentFromReader(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if width, _ := page.Find("meta[name='cover-max-width']").Attr("content"); width != "600" {
				t.Fatalf("cover width = %q", width)
			}
			if path == "/documents/john-doe-test-epub" {
				if src, _ := page.Find("img.cover").First().Attr("src"); src != "/documents/john-doe-test-epub/cover" {
					t.Fatalf("EPUB cover src = %q", src)
				}
				return
			}
			img := page.Find("img[data-pdf-src='/documents/john-doe-test-pdf/download']").First()
			if img.Length() != 1 {
				t.Fatal("missing PDF page-rendered cover")
			}
			if src, _ := img.Attr("src"); src == "/documents/john-doe-test-pdf/cover" {
				t.Fatal("PDF still loads extracted embedded images")
			}
			_, eager := img.Attr("data-cover-eager")
			if path == "/documents/john-doe-test-pdf" && !eager {
				t.Fatalf("eager = %v for %s", eager, path)
			}
			if !eager {
				if loading, _ := img.Attr("loading"); loading != "lazy" {
					t.Fatalf("loading = %q, want lazy", loading)
				}
			}
		})
	}
}

func TestPDFDownloadRangesAndLegacyCoverCache(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	fs := loadFilesInMemoryFs([]string{"testdata/library/metadata.pdf"})
	cfg := defaultTestConfig()
	cfg.CacheDir = "cache"
	if err := fs.MkdirAll("cache/covers", 0755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "cache/covers/john-doe-test-pdf.webp", []byte("wrong interior image"), 0644); err != nil {
		t.Fatal(err)
	}
	app := bootstrapApp(db, &infrastructure.NoEmail{}, fs, cfg)
	data, err := afero.ReadFile(fs, "testdata/library/metadata.pdf")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		path   string
		rangeH string
		status int
		body   []byte
	}{
		{"full PDF", "/download", "", http.StatusOK, data},
		{"first bytes", "/download", "bytes=0-9", http.StatusPartialContent, data[:10]},
		{"suffix", "/download", "bytes=-10", http.StatusPartialContent, data[len(data)-10:]},
		{"invalid range", "/download", "bytes=999999999-", http.StatusRequestedRangeNotSatisfiable, nil},
		{"stale image must not be served", "/cover", "", http.StatusNotFound, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, "/documents/john-doe-test-pdf"+tc.path, nil)
			if tc.rangeH != "" {
				req.Header.Set("Range", tc.rangeH)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.status)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if tc.body != nil && !bytes.Equal(body, tc.body) {
				t.Fatal("incorrect PDF response bytes")
			}
			if tc.status == http.StatusPartialContent {
				if resp.Header.Get("Content-Range") == "" || resp.Header.Get("Accept-Ranges") != "bytes" {
					t.Fatal("missing range response headers")
				}
				if resp.Header.Get("Content-Type") != "application/pdf" {
					t.Fatal("incorrect PDF content type")
				}
			}
		})
	}

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/documents/john-doe-test-pdf/download", nil))
	if err != nil {
		t.Fatal(err)
	}
	etag := response.Header.Get("ETag")
	_ = response.Body.Close()
	if etag == "" {
		t.Fatal("document response is missing ETag")
	}

	request := httptest.NewRequest(http.MethodGet, "/documents/john-doe-test-pdf/download", nil)
	request.Header.Set("If-None-Match", etag)
	response, err = app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional download status = %d, want %d", response.StatusCode, http.StatusNotModified)
	}
}
