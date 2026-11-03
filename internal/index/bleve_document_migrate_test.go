package index

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/metadata"
)

func legacyDocumentsIndex(t *testing.T, path, version string) bleve.Index {
	t.Helper()
	m := CreateDocumentsMapping().(*mapping.IndexMappingImpl)
	mappings := []*mapping.DocumentMapping{m.DefaultMapping}
	for _, documentMapping := range m.TypeMapping {
		mappings = append(mappings, documentMapping)
	}
	for _, documentMapping := range mappings {
		delete(documentMapping.Properties, "DataSources")
		delete(documentMapping.Properties, "EnrichedDataSources")
		for _, field := range []string{"DataSourceLink", "DataSourceScoreCount", "GoodReadsResourceID"} {
			stored := bleve.NewKeywordFieldMapping()
			stored.Index, stored.IncludeInAll, stored.DocValues = false, false, false
			documentMapping.AddFieldMappingsAt(field, stored)
		}
		storedNumber := bleve.NewNumericFieldMapping()
		storedNumber.Index, storedNumber.IncludeInAll, storedNumber.DocValues = false, false, false
		documentMapping.AddFieldMappingsAt("GoodReadsLegacyBookID", storedNumber)
		documentMapping.AddFieldMappingsAt("GoodReadsEnriched", bleve.NewBooleanFieldMapping())
	}
	idx, err := bleve.New(path, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.SetInternal(internalVersion, []byte(version)); err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestOpenDocumentsIndexMigratesStoredMetadata(t *testing.T) {
	for _, version := range previousDocumentVersions {
		t.Run(version, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "documents")
			legacy := legacyDocumentsIndex(t, path, version)
			want := make(map[string]Document)
			for _, data := range []biblioreads.DocumentData{
				{URL: "/book/show/123", RatingCount: "100", ResourceID: "work", LegacyBookID: 123, Enriched: true},
				{Enriched: true},
				{},
			} {
				id := "linked"
				if data.URL == "" {
					id = "pending"
					if data.Enriched {
						id = "unmatched"
					}
				}
				document := Document{
					ID: id, Slug: id,
					Metadata:     metadata.Metadata{Title: "Book " + id, Authors: []string{"Writer"}, Language: "es", Format: "EPUB"},
					AuthorsSlugs: []string{"writer"}, AddedOn: time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC),
					TextRankWords: []string{"atomic", "history"}, TextRankPhrases: []string{"atomic history"}, TextRankEnriched: true,
					DataSourceScoreValue: 4.25,
				}
				if err := legacy.Index(id, struct {
					Document
					DataSourceLink        string
					DataSourceScoreCount  string
					GoodReadsResourceID   string
					GoodReadsLegacyBookID int
					GoodReadsEnriched     bool
				}{document, data.URL, data.RatingCount, data.ResourceID, data.LegacyBookID, data.Enriched}); err != nil {
					t.Fatal(err)
				}
				reader := NewBleve(legacy, nil, nil, "", nil, Config{})
				stored, err := reader.documentByIndexID(id)
				if err != nil {
					t.Fatal(err)
				}
				if data != (biblioreads.DocumentData{}) {
					stored = withGoodReadsData(t, stored, data)
				}
				want[id] = stored
			}
			internal := map[string]string{
				string(internalLanguages): "es,en", string(internalIllustratedMinSize): "0.25",
				string(internalMinOccurrenceRatio): "0.1", string(internalMaxTextRankWords): "10000",
				string(internalDocCountAtLastPrune): "3",
			}
			for key, value := range internal {
				if err := legacy.SetInternal([]byte(key), []byte(value)); err != nil {
					t.Fatal(err)
				}
			}
			if err := legacy.Close(); err != nil {
				t.Fatal(err)
			}
			upgraded, err := OpenDocumentsIndex(path, 1)
			if err != nil {
				t.Fatal(err)
			}
			if err := upgraded.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenDocumentsIndex(path, 1)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			idx := NewBleve(reopened, nil, nil, "", nil, Config{})
			for id, expected := range want {
				got, err := idx.documentByIndexID(id)
				if err != nil || !reflect.DeepEqual(got, expected) {
					t.Fatalf("migration changed document:\ngot: %#v\nwant: %#v\nerror: %v", got, expected, err)
				}
			}
			if _, total, err := idx.documentsNeedingGoodReads(10); err != nil || total != 1 {
				t.Fatalf("migration changed enrichment queue: %d pending, %v", total, err)
			}
			for key, expected := range internal {
				got, err := reopened.GetInternal([]byte(key))
				if err != nil || string(got) != expected {
					t.Fatalf("migration lost internal metadata %s: %q, %v", key, got, err)
				}
			}
			if _, err := os.Stat(path + "." + version + "-backup"); !os.IsNotExist(err) {
				t.Fatalf("migration backup was not removed: %v", err)
			}
		})
	}
}

func TestOpenDocumentsIndexPreservesOriginalOnFailure(t *testing.T) {
	for _, version := range []string{"v16", "unsupported"} {
		t.Run(version, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "documents")
			legacy := legacyDocumentsIndex(t, path, version)
			if err := legacy.Index("saved", Document{ID: "saved"}); err != nil {
				t.Fatal(err)
			}
			if err := legacy.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenDocumentsIndex(path, 0); err == nil {
				t.Fatal("expected invalid batch size or unsupported version error")
			}
			original, err := bleve.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer original.Close()
			if count, err := original.DocCount(); err != nil || count != 1 {
				t.Fatalf("original index lost: %d, %v", count, err)
			}
			if got, err := original.GetInternal(internalVersion); err != nil || string(got) != version {
				t.Fatalf("original version changed: %q, %v", got, err)
			}
		})
	}
}

func TestOpenDocumentsIndexRecoversInterruptedSwap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "documents")
	legacy := legacyDocumentsIndex(t, path+".v16-backup", "v16")
	if err := legacy.Index("saved", Document{ID: "saved", Slug: "saved", Metadata: metadata.Metadata{Title: "Saved"}}); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := OpenDocumentsIndex(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if count, err := upgraded.DocCount(); err != nil || count != 1 {
		t.Fatalf("interrupted migration lost documents: %d, %v", count, err)
	}
}
