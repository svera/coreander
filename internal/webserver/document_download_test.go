package webserver_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
)

func TestDocumentDownloadDetectsChangesWithSameSizeAndModTime(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	fs := loadFilesInMemoryFs([]string{"testdata/library/metadata.pdf", "testdata/library/metadata.epub"})
	app := bootstrapApp(db, &infrastructure.NoEmail{}, fs, defaultTestConfig())
	t.Cleanup(func() { _ = app.Shutdown() })

	for _, format := range []string{"pdf", "epub"} {
		t.Run(format, func(t *testing.T) {
			path := "testdata/library/metadata." + format
			info, err := fs.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			data, err := afero.ReadFile(fs, path)
			if err != nil {
				t.Fatal(err)
			}
			oldETag := fmt.Sprintf(`"%x"`, sha256.Sum256(data))
			data[len(data)-1] ^= 1
			if err := afero.WriteFile(fs, path, data, info.Mode()); err != nil {
				t.Fatal(err)
			}
			if err := fs.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
				t.Fatal(err)
			}
			newETag := fmt.Sprintf(`"%x"`, sha256.Sum256(data))
			for _, tc := range []struct {
				etag   string
				status int
			}{
				{oldETag, http.StatusOK},
				{newETag, http.StatusNotModified},
			} {
				request := httptest.NewRequest(http.MethodGet, "/documents/john-doe-test-"+format+"/download", nil)
				request.Header.Set("If-None-Match", tc.etag)
				response, err := app.Test(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				if response.StatusCode != tc.status || response.Header.Get("ETag") != newETag {
					t.Fatalf("status/ETag = %d/%q, want %d/%q", response.StatusCode, response.Header.Get("ETag"), tc.status, newETag)
				}
				body, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if tc.status == http.StatusOK && !bytes.Equal(body, data) {
					t.Fatal("response did not contain the modified document")
				}
				if tc.status == http.StatusNotModified && len(body) != 0 {
					t.Fatal("304 response contained a body")
				}
			}
		})
	}
}

func TestPDFDownloadRanges(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	fs := loadFilesInMemoryFs([]string{"testdata/library/metadata.pdf"})
	cfg := defaultTestConfig()
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
}

func TestDocumentDownloadConditionalRequest(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	app := bootstrapApp(db, &infrastructure.NoEmail{},
		loadFilesInMemoryFs([]string{"testdata/library/metadata.pdf", "testdata/library/metadata.epub"}), defaultTestConfig())
	t.Cleanup(func() { _ = app.Shutdown() })

	for _, format := range []string{"pdf", "epub"} {
		for _, encoding := range []string{"identity", "gzip", "br", "gzip, deflate, br, zstd"} {
			t.Run(format+"/"+encoding, func(t *testing.T) {
				path := "/documents/john-doe-test-" + format + "/download"
				request := httptest.NewRequest(http.MethodGet, path, nil)
				request.Header.Set("Accept-Encoding", encoding)
				response, err := app.Test(request)
				if err != nil {
					t.Fatal(err)
				}
				etag := response.Header.Get("ETag")
				_ = response.Body.Close()
				if response.StatusCode != http.StatusOK {
					t.Fatalf("download status = %d, want %d", response.StatusCode, http.StatusOK)
				}
				if etag == "" {
					t.Fatal("document response is missing ETag")
				}
				if got := response.Header.Get("Content-Encoding"); got != "" {
					t.Errorf("document response was transformed using %q", got)
				}

				request = httptest.NewRequest(http.MethodGet, path, nil)
				request.Header.Set("Accept-Encoding", encoding)
				request.Header.Set("If-None-Match", etag)
				response, err = app.Test(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusNotModified {
					t.Fatalf("conditional download status = %d, want %d", response.StatusCode, http.StatusNotModified)
				}
				if got := response.Header.Get("ETag"); got != etag {
					t.Errorf("conditional response ETag = %q, want %q", got, etag)
				}
				body, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if len(body) != 0 {
					t.Errorf("304 response body has %d bytes, want 0", len(body))
				}
			})
		}
	}
}
