package webserver_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
)

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
		t.Run(format, func(t *testing.T) {
			path := "/documents/john-doe-test-" + format + "/download"
			response, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
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

			request := httptest.NewRequest(http.MethodGet, path, nil)
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
