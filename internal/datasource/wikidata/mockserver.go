package wikidata

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gosimple/slug"
)

func NewMockServer(t *testing.T, fixturePath string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/w/api.php") {
			queryValues := r.URL.Query()
			if queryValues.Get("action") == "wbsearchentities" {
				slug := slug.Make(queryValues.Get("search"))
				returnResponse(fmt.Sprintf("wbsearchentities-%s", slug), w, fixturePath)
				return
			}
			if queryValues.Get("action") == "wbgetentities" {
				id := queryValues.Get("ids")
				returnResponse(fmt.Sprintf("wbgetentities-%s", id), w, fixturePath)
				return
			}
		}
		t.Errorf("Expected to request '/w/api.php', got: %s", r.URL.Path)
	}))
}

func returnResponse(fixture string, w http.ResponseWriter, fixturePath string) {
	w.WriteHeader(http.StatusOK)
	filePath := fmt.Sprintf("%s/%s.json", fixturePath, fixture)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if strings.HasPrefix(fixture, "wbsearchentities-") {
			filePath = fmt.Sprintf("%s/wbsearchentities-no-results.json", fixturePath)
		}
		if strings.HasPrefix(fixture, "wbgetentities-") {
			filePath = fmt.Sprintf("%s/wbgetentities-no-such-entity.json", fixturePath)
		}
	}
	contents, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Couldn't read contents of %s", filePath)
	}
	if _, err = w.Write(contents); err != nil {
		log.Fatalf("Couldn't write contents of %s", filePath)
	}
}
