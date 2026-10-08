package webserver_test

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
)

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
