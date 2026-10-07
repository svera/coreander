package document

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

type readerTitleIndex struct {
	IdxReaderWriter
	document index.Document
}

func (idx readerTitleIndex) Document(string) (index.Document, error) {
	return idx.document, nil
}

type readerTitleViews struct {
	binding fiber.Map
}

func (*readerTitleViews) Load() error { return nil }

func (v *readerTitleViews) Render(_ io.Writer, _ string, binding any, _ ...string) error {
	v.binding = binding.(fiber.Map)
	return nil
}

func TestReaderTitle(t *testing.T) {
	for _, tc := range []struct {
		name    string
		authors []string
		title   string
		author  string
	}{
		{name: "no authors", title: "Generated title"},
		{name: "empty author", authors: []string{""}, title: "Generated title"},
		{name: "multiple empty authors", authors: []string{"", ""}, title: "Generated title"},
		{name: "whitespace authors", authors: []string{" ", "\t\n"}, title: "Generated title"},
		{name: "one author", authors: []string{"Jane Doe"}, title: "Jane Doe - Generated title", author: "Jane Doe"},
		{name: "multiple authors", authors: []string{"Jane Doe", "John Doe"}, title: "Jane Doe, John Doe - Generated title", author: "Jane Doe, John Doe"},
		{name: "mixed authors", authors: []string{"", " Jane Doe ", "\t"}, title: "Jane Doe - Generated title", author: "Jane Doe"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			views := &readerTitleViews{}
			app := fiber.New(fiber.Config{Views: views})
			t.Cleanup(func() { _ = app.Shutdown() })
			controller := &Controller{idx: readerTitleIndex{document: index.Document{
				Slug: "generated-title",
				Metadata: metadata.Metadata{
					Title:   "Generated title",
					Authors: tc.authors,
				},
			}}}
			app.Get("/documents/:slug/read", controller.Reader)

			response, err := app.Test(httptest.NewRequest(http.MethodGet, "/documents/generated-title/read", nil))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
			}
			if got := views.binding["Title"]; got != tc.title {
				t.Errorf("Title = %q, want %q", got, tc.title)
			}
			if got := views.binding["Author"]; got != tc.author {
				t.Errorf("Author = %q, want %q", got, tc.author)
			}
		})
	}
}
