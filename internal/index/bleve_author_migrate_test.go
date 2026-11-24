package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v5/internal/precisiondate"
)

func legacyAuthorsIndex(t *testing.T, path string) bleve.Index {
	t.Helper()
	m := CreateAuthorsMapping().(*mapping.IndexMappingImpl)
	delete(m.DefaultMapping.Properties, "DataSourceReferences")
	m.DefaultMapping.AddFieldMappingsAt("DataSourceID", bleve.NewKeywordFieldMapping())
	idx, err := bleve.New(path, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.SetInternal(internalVersion, []byte("2")); err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestOpenAuthorsIndexMigratesStoredMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authors_index")
	legacy := legacyAuthorsIndex(t, path)
	retrieved := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	want := Author{
		Slug: "alice-borchardt", Name: "Alice Borchardt", BirthName: "Alice O'Brien",
		RetrievedOn: retrieved,
		DataSourceReferences: map[string]string{
			"wikidata":     "Q1",
			"wikipedia:en": "https://en.wikipedia.org/wiki/Alice_Borchardt",
			"wikipedia:es": "https://es.wikipedia.org/wiki/Alice_Borchardt",
			"website":      "https://example.org",
		},
		Description: map[string]string{"en": "Writer", "es": "Escritora"}, InstanceOf: 5,
		DateOfBirth:     precisiondate.PrecisionDate{Date: date.New(1939, 10, 6), Precision: precisiondate.PrecisionDay},
		DateOfDeath:     precisiondate.PrecisionDate{Date: date.New(2007, 7, 24), Precision: precisiondate.PrecisionDay},
		DataSourceImage: "image.jpg", Gender: 6581072,
		Pseudonyms: []string{"Alice", "A. Borchardt"}, DocumentCount: 7,
	}
	for _, author := range []Author{want, {Slug: "other", Name: "Other", RetrievedOn: retrieved}, {Slug: "third", Name: "Third", RetrievedOn: retrieved}} {
		stored := author
		stored.DataSourceReferences = nil
		if err := legacy.Index(author.Slug, struct {
			Author
			DataSourceID  string
			Website       string
			WikipediaLink map[string]string
		}{stored, author.DataSourceReferences["wikidata"], author.DataSourceReferences["website"], map[string]string{
			"en": author.DataSourceReferences["wikipedia:en"], "es": author.DataSourceReferences["wikipedia:es"],
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := OpenAuthorsIndex(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	documents, err := bleve.NewMemOnly(CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	idx := NewBleve(documents, upgraded, nil, "", nil, Config{})
	got, err := idx.Author(want.Slug, "en")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("migration changed author metadata:\ngot: %#v\nwant: %#v", got, want)
	}
	if count, err := idx.TotalAuthors(); err != nil || count != 3 {
		t.Fatalf("expected all 3 authors migrated, got %d, %v", count, err)
	}
	if pending, err := idx.AuthorsWithoutInfo(); err != nil || len(pending) != 0 {
		t.Fatalf("migration must not trigger new Wikidata lookups: %+v, %v", pending, err)
	}
	if version, err := upgraded.GetInternal(internalVersion); err != nil || string(version) != AuthorVersion {
		t.Fatalf("unexpected migrated version %q, %v", version, err)
	}
	if _, err := os.Stat(path + ".v2-backup"); !os.IsNotExist(err) {
		t.Fatalf("expected original index removed after successful migration, got %v", err)
	}

	got.DataSourceReferences["goodreads"] = "1.Writer"
	if err := idx.IndexAuthor(got); err != nil {
		t.Fatal(err)
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenAuthorsIndex(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	req := bleve.NewSearchRequest(bleve.NewDocIDQuery([]string{want.Slug}))
	req.Fields = []string{"*"}
	result, err := reopened.Search(req)
	if err != nil || len(result.Hits) != 1 {
		t.Fatalf("reading reopened author: %v", err)
	}
	want.DataSourceReferences["goodreads"] = "1.Writer"
	if got := hydrateAuthor(result.Hits[0]); !reflect.DeepEqual(got, want) {
		t.Fatalf("migrated mapping or URL did not survive reopening: %#v", got)
	}
}

func TestOpenAuthorsIndexRecoversInterruptedSwap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authors_index")
	legacy := legacyAuthorsIndex(t, path+".v2-backup")
	if err := legacy.Index("saved", map[string]string{"Slug": "saved", "Name": "Saved Author", "DataSourceID": "Q1"}); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := OpenAuthorsIndex(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if count, err := upgraded.DocCount(); err != nil || count != 1 {
		t.Fatalf("expected saved author recovered, got %d, %v", count, err)
	}
}

func TestOpenAuthorsIndexMigratesVersion3SourceID(t *testing.T) {
	for _, referenceID := range []string{"", "Q2"} {
		t.Run("reference="+referenceID, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "authors_index")
			m := oldReferenceAuthorsMapping()
			m.DefaultMapping.AddFieldMappingsAt("DataSourceID", bleve.NewKeywordFieldMapping())
			legacy, err := bleve.New(path, m)
			if err != nil {
				t.Fatal(err)
			}
			want := Author{
				Slug: "writer", Name: "Writer", RetrievedOn: time.Now().UTC().Truncate(time.Second),
				Description: map[string]string{"en": "Writer", "es": "Escritora"}, DocumentCount: 7,
				Pseudonyms: []string{"Writer Alias"},
				DataSourceReferences: map[string]string{
					"goodreads": "1.Writer", "wikipedia:es": "https://es.wikipedia.org/wiki/Writer",
				},
				Bio: AuthorBio{Source: "https://es.wikipedia.org/wiki/Writer", Language: "es",
					Text: map[string]string{"es": "<b>Biografia</b>", "en": "<b>Biography</b>"}},
			}
			if referenceID != "" {
				want.DataSourceReferences["wikidata"] = referenceID
			}
			links, err := json.Marshal(want.DataSourceReferences)
			if err != nil {
				t.Fatal(err)
			}
			stored := want
			stored.DataSourceReferences = nil
			if err := legacy.Index(want.Slug, struct {
				Author
				DataSourceID    string
				DataSourceLinks string
			}{stored, "Q1", string(links)}); err != nil {
				t.Fatal(err)
			}
			if err := legacy.SetInternal(internalVersion, []byte("3")); err != nil {
				t.Fatal(err)
			}
			if err := legacy.Close(); err != nil {
				t.Fatal(err)
			}
			upgraded, err := OpenAuthorsIndex(path, 1)
			if err != nil {
				t.Fatal(err)
			}
			if err := upgraded.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenAuthorsIndex(path, 1)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			req := bleve.NewSearchRequest(bleve.NewDocIDQuery([]string{want.Slug}))
			req.Fields = []string{"*"}
			result, err := reopened.Search(req)
			if err != nil || len(result.Hits) != 1 {
				t.Fatalf("reading migrated author: %v", err)
			}
			if _, found := result.Hits[0].Fields["DataSourceID"]; found {
				t.Fatal("migration retained the standalone source ID")
			}
			if _, found := result.Hits[0].Fields["DataSourceLinks"]; found {
				t.Fatal("migration retained the old reference field")
			}
			if _, found := result.Hits[0].Fields["DataSourceReferences"]; !found {
				t.Fatal("migration did not store the renamed reference field")
			}
			if referenceID == "" {
				want.DataSourceReferences["wikidata"] = "Q1"
			}
			if got := hydrateAuthor(result.Hits[0]); !reflect.DeepEqual(got, want) {
				t.Fatalf("migration changed author metadata or references:\ngot: %#v\nwant: %#v", got, want)
			}
		})
	}
}

func oldReferenceAuthorsMapping() *mapping.IndexMappingImpl {
	m := CreateAuthorsMapping().(*mapping.IndexMappingImpl)
	m.DefaultMapping.Properties["DataSourceLinks"] = m.DefaultMapping.Properties["DataSourceReferences"]
	delete(m.DefaultMapping.Properties, "DataSourceReferences")
	return m
}

func TestOpenAuthorsIndexMigratesDevelopmentVersion4(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authors_index")
	authors, err := bleve.New(path, oldReferenceAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	if err := authors.SetInternal(internalVersion, []byte(AuthorVersion)); err != nil {
		t.Fatal(err)
	}
	if err := authors.Index("writer", struct {
		Author
		DataSourceLinks string
	}{
		Author: Author{Slug: "writer", Name: "Writer", DocumentCount: 3,
			Bio: AuthorBio{Source: "1.Writer", Language: "en", Text: map[string]string{"en": "Biography"}}},
		DataSourceLinks: `{"wikidata":"Q1","website":"https://example.org"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := authors.Close(); err != nil {
		t.Fatal(err)
	}
	authors, err = OpenAuthorsIndex(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	idx := NewBleve(nil, authors, nil, "", nil, Config{})
	author, err := idx.Author("writer", "")
	if err != nil || author.DataSourceReferences["wikidata"] != "Q1" || author.DataSourceReferences["website"] != "https://example.org" ||
		author.DocumentCount != 3 || author.Bio.Text["en"] != "Biography" {
		t.Fatalf("existing references lost: %+v, %v", author, err)
	}
	author.DataSourceReferences["goodreads"] = "1.Writer"
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	if err := authors.Close(); err != nil {
		t.Fatal(err)
	}
	authors, err = OpenAuthorsIndex(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer authors.Close()
	req := bleve.NewSearchRequest(bleve.NewDocIDQuery([]string{"writer"}))
	req.Fields = []string{"*"}
	result, err := authors.Search(req)
	if err != nil || len(result.Hits) != 1 {
		t.Fatalf("reading saved author: %v", err)
	}
	fields := result.Hits[0].Fields
	if _, ok := fields["DataSourceReferences"].(string); !ok {
		t.Fatal("references must use the renamed stored field")
	}
	if _, ok := fields["DataSourceLinks"]; ok {
		t.Fatal("migration retained the old reference field")
	}
	got := hydrateAuthor(result.Hits[0])
	if !reflect.DeepEqual(got.DataSourceReferences, author.DataSourceReferences) {
		t.Fatalf("references did not survive reopening: %+v", got.DataSourceReferences)
	}
}

func TestOpenAuthorsIndexRemovesPreviouslyRetainedBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authors_index")
	backup := legacyAuthorsIndex(t, path+".v2-backup")
	if err := backup.Close(); err != nil {
		t.Fatal(err)
	}
	current := CreateAuthorsIndex(path)
	if err := indexAuthor(current, "saved", Author{Slug: "saved", Name: "Saved Author", DataSourceReferences: map[string]string{"wikidata": "Q1"}}); err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenAuthorsIndex(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if count, err := reopened.DocCount(); err != nil || count != 1 {
		t.Fatalf("current index changed while removing backup: %d, %v", count, err)
	}
	if _, err := os.Stat(path + ".v2-backup"); !os.IsNotExist(err) {
		t.Fatalf("expected previously retained backup removed, got %v", err)
	}
}

func TestOpenAuthorsIndexMigrationFailurePreservesOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authors_index")
	legacy := legacyAuthorsIndex(t, path)
	if err := legacy.Index("saved", map[string]string{"Slug": "saved", "Name": "Saved Author", "DataSourceID": "Q1"}); err != nil {
		t.Fatal(err)
	}

	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenAuthorsIndex(path, 0); err == nil {
		t.Fatal("expected invalid batch size to fail migration")
	}
	original, err := bleve.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	if version, err := original.GetInternal(internalVersion); err != nil || string(version) != "2" {
		t.Fatalf("original version changed after migration failure: %q, %v", version, err)
	}
	if count, err := original.DocCount(); err != nil || count != 1 {
		t.Fatalf("original metadata lost after migration failure: %d, %v", count, err)
	}
}

func TestOpenAuthorsIndexMigratesLegacyAuthorLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authors_index")
	m := CreateAuthorsMapping().(*mapping.IndexMappingImpl)
	delete(m.DefaultMapping.Properties, "DataSourceReferences")
	m.DefaultMapping.AddFieldMappingsAt("DataSourceID", bleve.NewKeywordFieldMapping())
	legacyURL := bleve.NewKeywordFieldMapping()
	legacyURL.Index = false
	legacyURL.IncludeInAll = false
	legacyURL.DocValues = false
	m.DefaultMapping.AddFieldMappingsAt("Website", legacyURL)
	legacy, err := bleve.New(path, m)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]any{
		"Slug": "alice", "Name": "Alice",
		"DataSourceID": "Q1", "DocumentCount": 7,
		"Website": "https://example.org",
		"WikipediaLink": map[string]string{
			"en": "https://en.wikipedia.org/wiki/Alice",
			"es": "https://es.wikipedia.org/wiki/Alice",
		},
	}
	if err := legacy.Index("alice", fields); err != nil {
		t.Fatal(err)
	}
	if err := legacy.SetInternal(internalVersion, []byte("2")); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := OpenAuthorsIndex(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	idx := NewBleve(nil, upgraded, nil, "", nil, Config{})
	author, err := idx.Author("alice", "")
	if err != nil || author.DataSourceReferences["wikidata"] != "Q1" || author.DocumentCount != 7 ||
		author.DataSourceReferences["website"] != "https://example.org" ||
		author.DataSourceReferences["wikipedia:en"] != "https://en.wikipedia.org/wiki/Alice" ||
		author.DataSourceReferences["wikipedia:es"] != "https://es.wikipedia.org/wiki/Alice" {
		t.Fatalf("migration lost legacy links or metadata: %+v, %v", author, err)
	}
	author.DataSourceReferences["other"] = "https://example.org/alice"
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	if err := upgraded.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenAuthorsIndex(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	req := bleve.NewSearchRequest(bleve.NewDocIDQuery([]string{"alice"}))
	req.Fields = []string{"*"}
	result, err := reopened.Search(req)
	if err != nil || len(result.Hits) != 1 {
		t.Fatalf("reading reopened author: %v", err)
	}
	got := hydrateAuthor(result.Hits[0])
	if !reflect.DeepEqual(got, author) {
		t.Fatalf("links or metadata changed after reopening:\ngot: %#v\nwant: %#v", got, author)
	}
	for field := range result.Hits[0].Fields {
		if field == "Website" || strings.HasPrefix(field, "WikipediaLink.") {
			t.Fatalf("migrated author still stores legacy field %s", field)
		}
	}
	if _, err := os.Stat(path + ".v2-backup"); !os.IsNotExist(err) {
		t.Fatalf("migration backup was not removed: %v", err)
	}
}
