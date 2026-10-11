package webserver_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
)

func TestReaderOfflineWorker(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	cfg := defaultTestConfig()
	cfg.RequireAuth = true
	app := bootstrapApp(db, &infrastructure.NoEmail{}, afero.NewMemMapFs(), cfg)
	request, err := http.NewRequest(http.MethodGet, "/reader-service-worker.js", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Worker must be public: got %d", response.StatusCode)
	}
	if response.Header.Get("Cache-Control") != "no-cache" {
		t.Fatal("Worker updates must not be served from an immutable HTTP cache")
	}
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/javascript") {
		t.Fatal("Worker must have a JavaScript content type")
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range []string{
		"/css/reader.css", "/js/reader.js", "/js/reader-offline.js",
		"/js/reader-popup.js",
		"/js/foliate-js/epub.js", "/js/foliate-js/vendor/pdfjs/pdf.worker.mjs",
		"/js/foliate-js/vendor/pdfjs/text_layer_builder.css", "/js/foliate-js/ui/tree.js",
	} {
		if !strings.Contains(string(body), `"`+asset+`"`) {
			t.Errorf("Missing offline reader asset: %s", asset)
		}
	}
	for _, excluded := range []string{"/js/reader-service-worker.js", "/js/foliate-js/tests/", "/js/foliate-js/rollup/"} {
		if strings.Contains(string(body), excluded) {
			t.Errorf("Unexpected offline reader asset: %s", excluded)
		}
	}
	if strings.Contains(string(body), "__READER_CONFIG__") {
		t.Fatal("Unexpanded worker configuration")
	}
}
