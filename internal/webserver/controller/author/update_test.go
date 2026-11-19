package author

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/index"
)

type alternativeAuthorSource struct {
	DataSource
	retrievedIDs []string
}

func (*alternativeAuthorSource) NormalizeAuthorID(value string) (string, error) {
	if value != "https://example.org/authors/alice" {
		return "", fmt.Errorf("invalid profile")
	}
	return "alice", nil
}

func (s *alternativeAuthorSource) RetrieveAuthor(ids []string, _ []string) (model.Author, error) {
	s.retrievedIDs = ids
	return nil, nil
}

type existingAuthorIndex struct{ IdxReader }

func (existingAuthorIndex) Author(slug, _ string) (index.Author, error) {
	return index.Author{Slug: slug}, nil
}

func TestUpdateUsesDataSourceProfileParser(t *testing.T) {
	source := &alternativeAuthorSource{}
	controller := &Controller{dataSource: source, idx: existingAuthorIndex{}}
	app := fiber.New()
	app.Put("/authors/:slug", func(c fiber.Ctx) error {
		c.Locals("Lang", "en")
		c.Locals("SupportedLanguages", []string{"en"})
		return controller.Update(c)
	})

	for _, tc := range []struct {
		input      string
		wantStatus int
	}{
		{"https://example.org/authors/alice", http.StatusNotFound},
		{"invalid", http.StatusBadRequest},
	} {
		source.retrievedIDs = nil
		req := httptest.NewRequest(http.MethodPut, "/authors/test", strings.NewReader(url.Values{"sourceID": {tc.input}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.wantStatus {
			t.Fatalf("%q: status = %d, want %d", tc.input, resp.StatusCode, tc.wantStatus)
		}
		if tc.wantStatus == http.StatusNotFound && (len(source.retrievedIDs) != 1 || source.retrievedIDs[0] != "alice") {
			t.Fatalf("normalized source ID was not used for retrieval: %v", source.retrievedIDs)
		}
		if tc.wantStatus == http.StatusBadRequest && len(source.retrievedIDs) != 0 {
			t.Fatal("invalid input must not trigger retrieval")
		}
	}
}
