package webserver_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
)

// TestVersionParameterInURLs checks that static assets always carry a cache-busting
// query parameter, regardless of the release version. Cache-busting is driven by a
// per-process asset version, not the release version: a dev/dirty build's version
// string doesn't change between rebuilds unless committed, so tying cache-busting to
// it can leave a browser (mobile in particular, which has no "disable cache" escape
// hatch) stuck serving a stale immutable asset across multiple rebuilds of the same
// commit.
func TestVersionParameterInURLs(t *testing.T) {
	var testCases = []struct {
		name    string
		version string
	}{
		{name: "Version parameter is always present when version is set", version: "v1.2.3"},
		{name: "Version parameter is always present when version is empty", version: ""},
		{name: "Version parameter is always present when version is unknown", version: "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup app with specific version
			db := infrastructure.Connect(":memory:", 250)
			config := webserver.Config{
				Version:               tc.version,
				SessionTimeout:        24 * 60 * 60 * 1000000000, // 24 hours in nanoseconds
				RecoveryTimeout:       2 * 60 * 60 * 1000000000,  // 2 hours in nanoseconds
				LibraryPath:           "testdata/library",
				WordsPerMinute:        250,
				UploadDocumentMaxSize: 1,
			}
			app := bootstrapApp(db, &infrastructure.NoEmail{}, loadDirInMemoryFs("testdata/library"), config)

			// Make request to home page
			req, err := http.NewRequest(http.MethodGet, "/", nil)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			response, err := app.Test(req)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if response.StatusCode != http.StatusOK {
				t.Errorf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
			}

			// Parse HTML response
			doc, err := goquery.NewDocumentFromReader(response.Body)
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			// Check CSS links for a cache-busting parameter
			cssLinks := doc.Find("link[rel='stylesheet']")
			if cssLinks.Length() == 0 {
				t.Fatal("No CSS links found in the response")
			}

			cssLinks.Each(func(i int, s *goquery.Selection) {
				href, exists := s.Attr("href")
				if !exists {
					t.Error("CSS link missing href attribute")
					return
				}
				if !strings.Contains(href, "?v=") {
					t.Errorf("CSS link '%s' should contain a cache-busting parameter", href)
				}
			})

			// Check JavaScript links for a cache-busting parameter
			jsLinks := doc.Find("script[src]")
			if jsLinks.Length() == 0 {
				t.Fatal("No JavaScript links found in the response")
			}

			jsLinks.Each(func(i int, s *goquery.Selection) {
				src, exists := s.Attr("src")
				if !exists {
					t.Error("JavaScript link missing src attribute")
					return
				}
				if !strings.Contains(src, "?v=") {
					t.Errorf("JavaScript link '%s' should contain a cache-busting parameter", src)
				}
			})

			// Check image links for a cache-busting parameter
			imgLinks := doc.Find("img[src]")
			if imgLinks.Length() == 0 {
				t.Fatal("No image links found in the response")
			}

			imgLinks.Each(func(i int, s *goquery.Selection) {
				src, exists := s.Attr("src")
				if !exists {
					t.Error("Image link missing src attribute")
					return
				}

				// Only check images that are from the /images/ path (static assets)
				if strings.HasPrefix(src, "/images/") {
					if !strings.Contains(src, "?v=") {
						t.Errorf("Image link '%s' should contain a cache-busting parameter", src)
					}
				}
			})
		})
	}
}
