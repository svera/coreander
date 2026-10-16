package webserver_test

import (
	"encoding/xml"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/PuerkitoBio/goquery"
	"github.com/blevesearch/bleve/v2"
	"github.com/pirmd/epub"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/datasource/wikidata"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

func TestDocumentDetailAuthorUpdateWritesWikidataID(t *testing.T) {
	library := t.TempDir()
	filename := filepath.Join(library, "book.epub")
	data, err := os.ReadFile("fixtures/library/metadata.epub")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := epub.WriteMetadataToFile(filename, &epub.Information{
		Title:   []epub.MetadataValue{epub.NewMetadataValue("Book")},
		Creator: []epub.Author{{FullName: "John Doe", Role: "aut"}},
	}); err != nil {
		t.Fatal(err)
	}
	documents, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authors, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	appFS := afero.NewOsFs()
	idx := index.NewBleve(documents, authors, appFS, library,
		map[string]metadata.Reader{".epub": metadata.NewEpubReader()}, index.Config{})
	t.Cleanup(func() { _ = idx.Close() })
	if err := idx.AddLibrary(100, false, 1); err != nil {
		t.Fatal(err)
	}
	author, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	author.DataSourceID = "Q42"
	author.RetrievedOn = time.Now().UTC()
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}

	server := wikidata.NewMockServer(t, "fixtures/datasource/wikidata")
	t.Cleanup(server.Close)
	previousDomain := gowikidata.WikidataDomain
	gowikidata.WikidataDomain = server.URL
	t.Cleanup(func() { gowikidata.WikidataDomain = previousDomain })

	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	cfg := defaultTestConfig()
	cfg.LibraryPath = library
	cfg.CacheDir = t.TempDir()
	sender := &infrastructure.NoEmail{}
	source := wikidata.NewWikidataSource(wikidata.Gowikidata{})
	controllers := webserver.SetupControllers(cfg, db, idx, sender, appFS, source)
	app := webserver.New(cfg, controllers, sender, idx, &model.UserRepository{DB: db})
	t.Cleanup(func() { _ = app.Shutdown() })
	cookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	response, err := getRequest(cookie, app, "/documents/john-doe-book", t)
	if err != nil {
		t.Fatal(err)
	}
	page, err := goquery.NewDocumentFromReader(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("document detail status = %d", response.StatusCode)
	}
	form := page.Find("#update-author-form-john-doe form")
	action, ok := form.Attr("hx-put")
	if !ok || action != "/authors/john-doe?style=clear" {
		t.Fatalf("unexpected document-detail update action: %q", action)
	}
	if value, _ := form.Find("input[name=sourceID]").Attr("value"); value != "Q42" {
		t.Fatalf("initial Wikidata ID = %q, want Q42", value)
	}

	for _, tc := range []struct {
		name   string
		id     string
		status int
		want   string
	}{
		{"changing identity", "Q1234", http.StatusOK, "Q1234"},
		{"unknown identity leaves metadata unchanged", "Q999999999", http.StatusNotFound, "Q1234"},
		{"clearing identity", "", http.StatusOK, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, err := putRequest(url.Values{"sourceID": {tc.id}}, cookie, app, action, t)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tc.status {
				t.Fatalf("update status = %d, want %d", response.StatusCode, tc.status)
			}
			if tc.status == http.StatusOK {
				fragment, err := goquery.NewDocumentFromReader(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if value, _ := fragment.Find("input[name=sourceID]").Attr("value"); value != tc.want {
					t.Fatalf("updated form ID = %q, want %q", value, tc.want)
				}
			}
			info, err := epub.GetMetadataFromFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, attribute := range info.Creator[0].Attributes {
				if attribute.Name == (xml.Name{Space: metadata.CoreanderMetadataPrefix, Local: "wd_id"}) {
					count++
					if attribute.Value != tc.want {
						t.Fatalf("EPUB wd_id = %q, want %q", attribute.Value, tc.want)
					}
				}
			}
			if (tc.want == "" && count != 0) || (tc.want != "" && count != 1) {
				t.Fatalf("unexpected wd_id attribute count: %d", count)
			}
			stored, err := idx.Author("john-doe", "en")
			if err != nil {
				t.Fatal(err)
			}
			if stored.DataSourceID != tc.want {
				t.Fatalf("indexed ID = %q, want %q", stored.DataSourceID, tc.want)
			}
		})
	}
}
