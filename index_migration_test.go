package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

type migrationReader struct{ metadata.PdfReader }

func (migrationReader) Metadata(string) (metadata.Metadata, error) {
	return metadata.Metadata{Title: "Rebuilt book", Authors: []string{"Existing Author"}, Format: "PDF"}, nil
}

type legacyV13Document struct {
	metadata.Metadata
	ID                string
	Slug              string
	AuthorsSlugs      []string
	IllustratorsSlugs []string
	SeriesSlug        string
	SubjectsSlugs     []string
	AddedOn           time.Time
	TextRankPhrases   []string
	TextRankWords     []string
	TextRankEnriched  bool
}

func seedDocumentIndex(t *testing.T, version string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(homeDir, ".coreander"), 0755); err != nil {
		t.Fatal(err)
	}
	documentsMapping := index.CreateDocumentsMapping().(*mapping.IndexMappingImpl)
	delete(documentsMapping.DefaultMapping.Properties, "DataSources")
	delete(documentsMapping.DefaultMapping.Properties, "EnrichedDataSources")
	delete(documentsMapping.DefaultMapping.Properties, "DataSourceScoreValue")
	for _, documentMapping := range documentsMapping.TypeMapping {
		delete(documentMapping.Properties, "DataSources")
		delete(documentMapping.Properties, "EnrichedDataSources")
		delete(documentMapping.Properties, "DataSourceScoreValue")
	}
	legacyURLMapping := bleve.NewKeywordFieldMapping()
	legacyURLMapping.Index = false
	documentsMapping.DefaultMapping.AddFieldMappingsAt("GoodReadsBookURL", legacyURLMapping)
	documentsMapping.DefaultMapping.AddFieldMappingsAt("GoodReadsAuthorURL", legacyURLMapping)
	documentsMapping.DefaultMapping.AddFieldMappingsAt("GoodReadsEnriched", bleve.NewBooleanFieldMapping())
	documents, err := bleve.New(homeDir+documentsIndexPath, documentsMapping)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"version": version, "illustrated_min_size": "0.25",
		"min_occurrence_ratio": "0.1", "max_textrank_words": "10000",
	} {
		if err := documents.SetInternal([]byte(key), []byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := documents.Index("legacy-book", legacyV13Document{
		ID: "legacy-book", Slug: "legacy-book", AuthorsSlugs: []string{"existing-author"},
		Metadata: metadata.Metadata{Title: "Legacy book", Authors: []string{"Existing Author"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := documents.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestGetIndexesRebuildsOlderDocuments(t *testing.T) {
	for number := 1; number <= 13; number++ {
		for _, force := range []bool{false, true} {
			version := fmt.Sprintf("v%d", number)
			t.Run(fmt.Sprintf("%s/force=%t", version, force), func(t *testing.T) {
				previousHome, previousInput := homeDir, input
				t.Cleanup(func() { homeDir, input = previousHome, previousInput })
				homeDir = t.TempDir()
				input.BatchSize, input.ForceIndexing = 1, force
				seedDocumentIndex(t, version)
				authors := index.CreateAuthorsIndex(homeDir + authorsIndexPath)
				retrieved := time.Now().UTC().Truncate(time.Second)
				if err := authors.Index("existing-author", index.Author{
					Slug: "existing-author", Name: "Existing Author", DataSourceID: "Q1", RetrievedOn: retrieved,
				}); err != nil {
					t.Fatal(err)
				}
				if err := authors.Close(); err != nil {
					t.Fatal(err)
				}

				fs := afero.NewOsFs()
				lib := filepath.Join(homeDir, "library")
				if err := fs.MkdirAll(lib, 0755); err != nil {
					t.Fatal(err)
				}
				if err := afero.WriteFile(fs, filepath.Join(lib, "book.pdf"), []byte("book"), 0644); err != nil {
					t.Fatal(err)
				}
				documents, authors, needsReindex := getIndexes(fs, 0.25, 0.1, 10000)
				if version == "v13" && !force {
					if needsReindex {
						t.Fatal("v13 migration must not request library reindexing")
					}
					if count, err := documents.DocCount(); err != nil || count != 1 {
						t.Fatalf("v13 migration must preserve documents: count=%d, err=%v", count, err)
					}
					if migratedVersion, err := documents.GetInternal([]byte("version")); err != nil || string(migratedVersion) != index.DocumentVersion {
						t.Fatalf("expected migrated index version %q, got %q, %v", index.DocumentVersion, migratedVersion, err)
					}
					idx := index.NewBleve(documents, authors, fs, lib, nil, index.Config{})
					document, err := idx.Document("legacy-book")
					if err != nil || document.Title != "Legacy book" {
						t.Fatalf("v13 migration must preserve indexed document: %+v, %v", document, err)
					}
					data, err := index.DataSourceData[biblioreads.DocumentData](document, biblioreads.Source)
					if err != nil || data.URL != "" || data.Enriched {
						t.Fatalf("v13 migration must not introduce Goodreads data: %+v, %v", data, err)
					}
					if err := idx.Close(); err != nil {
						t.Fatal(err)
					}
					return
				}
				if !needsReindex {
					t.Fatal("older index upgrade must request forced library indexing")
				}
				if count, err := documents.DocCount(); err != nil || count != 0 {
					t.Fatalf("older documents must be rebuilt: count=%d, err=%v", count, err)
				}
				idx := index.NewBleve(documents, authors, fs, lib,
					map[string]metadata.Reader{".pdf": migrationReader{}},
					index.Config{IllustratedMinSize: 0.25, MinOccurrenceRatio: 0.1, MaxTextRankWords: 10000})
				if err := idx.AddLibrary(1, needsReindex || input.ForceIndexing, 1); err != nil {
					t.Fatal(err)
				}
				book, err := idx.Document("existing-author-rebuilt-book")
				if err != nil || book.Title != "Rebuilt book" {
					t.Fatalf("library must be searchable after rebuilding: %+v, %v", book, err)
				}
				author, err := idx.Author("existing-author", "")
				if err != nil || author.DataSourceID != "Q1" || !author.RetrievedOn.Equal(retrieved) {
					t.Fatalf("rebuilding must preserve author metadata: %+v, %v", author, err)
				}
				if err := idx.Close(); err != nil {
					t.Fatal(err)
				}

				documents, authors, needsReindex = getIndexes(fs, 0.25, 0.1, 10000)
				defer documents.Close()
				defer authors.Close()
				if needsReindex {
					t.Fatal("completed upgrade must not rebuild again on restart")
				}
				if version, err := documents.GetInternal([]byte("version")); err != nil || string(version) != index.DocumentVersion {
					t.Fatalf("expected persisted current version, got %q, %v", version, err)
				}
				if count, err := documents.DocCount(); err != nil || count != 1 {
					t.Fatalf("restart must retain rebuilt documents: count=%d, err=%v", count, err)
				}
			})
		}
	}
}

func TestGetIndexesRejectsUnsupportedDocuments(t *testing.T) {
	if childHome := os.Getenv("COREANDER_TEST_UNSUPPORTED_INDEX_HOME"); childHome != "" {
		homeDir = childHome
		input.BatchSize, input.ForceIndexing = 1, true
		getIndexes(afero.NewOsFs(), 0.25, 0.1, 10000)
		return
	}
	for _, version := range []string{"", "v0", "v15", "v99", "v01", "v-1", "13", "unknown"} {
		t.Run(version, func(t *testing.T) {
			previousHome := homeDir
			t.Cleanup(func() { homeDir = previousHome })
			homeDir = t.TempDir()
			seedDocumentIndex(t, version)
			command := exec.Command(os.Args[0], "-test.run=^TestGetIndexesRejectsUnsupportedDocuments$")
			command.Env = append(os.Environ(), "COREANDER_TEST_UNSUPPORTED_INDEX_HOME="+homeDir)
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), "Unsupported documents index version") {
				t.Fatalf("expected startup refusal, got %v: %s", err, output)
			}
			documents, err := bleve.Open(homeDir + documentsIndexPath)
			if err != nil {
				t.Fatal(err)
			}
			defer documents.Close()
			if stored, err := documents.GetInternal([]byte("version")); err != nil || string(stored) != version {
				t.Fatalf("unsupported version must remain untouched: %q, %v", stored, err)
			}
			if count, err := documents.DocCount(); err != nil || count != 1 {
				t.Fatalf("unsupported documents must remain untouched: count=%d, err=%v", count, err)
			}
		})
	}
}

func TestGetIndexesMigratesV13WithoutReindexWhenAuthorsIndexIsMissing(t *testing.T) {
	previousHome, previousInput := homeDir, input
	t.Cleanup(func() { homeDir, input = previousHome, previousInput })
	homeDir = t.TempDir()
	input.BatchSize, input.ForceIndexing = 1, false
	seedDocumentIndex(t, "v13")

	documents, authors, needsReindex := getIndexes(afero.NewOsFs(), 0.25, 0.1, 10000)
	defer documents.Close()
	defer authors.Close()
	if needsReindex {
		t.Fatal("creating the missing authors index must not force document reindexing")
	}
	if count, err := documents.DocCount(); err != nil || count != 1 {
		t.Fatalf("v13 migration must preserve documents: count=%d, err=%v", count, err)
	}
	if count, err := authors.DocCount(); err != nil || count != 1 {
		t.Fatalf("authors index must be rebuilt from migrated documents: count=%d, err=%v", count, err)
	}
}

func withBiblioReadsData(t *testing.T, document index.Document, data biblioreads.DocumentData) index.Document {
	t.Helper()
	if err := index.SetDataSourceData(&document, biblioreads.Source, data); err != nil {
		t.Fatal(err)
	}
	return document
}

func TestGetIndexesMigratesAuthorsWithoutReindexingDocuments(t *testing.T) {
	previousHome, previousInput := homeDir, input
	t.Cleanup(func() { homeDir, input = previousHome, previousInput })
	homeDir = t.TempDir()
	input.BatchSize = 1
	fs := afero.NewOsFs()
	if err := fs.MkdirAll(filepath.Join(homeDir, ".coreander"), 0755); err != nil {
		t.Fatal(err)
	}
	documents := index.CreateDocumentsIndex(homeDir + documentsIndexPath)
	for key, value := range map[string]string{
		"version":              index.DocumentVersion,
		"illustrated_min_size": "0.25",
		"min_occurrence_ratio": "0.1",
		"max_textrank_words":   "10000",
	} {
		if err := documents.SetInternal([]byte(key), []byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	doc := withBiblioReadsData(t, index.Document{
		ID: "book", Slug: "book", AuthorsSlugs: []string{"alice-borchardt"},
		Metadata: metadata.Metadata{Title: "La Reina Dragon", Authors: []string{"Alice Borchardt"}},
	}, biblioreads.DocumentData{Enriched: true})

	if err := index.IndexDocument(documents, doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	if err := documents.Close(); err != nil {
		t.Fatal(err)
	}
	legacyMapping := index.CreateAuthorsMapping().(*mapping.IndexMappingImpl)
	delete(legacyMapping.DefaultMapping.Properties, "DataSourceLinks")
	authors, err := bleve.New(homeDir+authorsIndexPath, legacyMapping)
	if err != nil {
		t.Fatal(err)
	}
	retrieved := time.Now().UTC().Truncate(time.Second)
	if err := authors.Index("alice-borchardt", index.Author{
		Slug: "alice-borchardt", Name: "Alice Borchardt", DataSourceID: "Q1", RetrievedOn: retrieved,
	}); err != nil {
		t.Fatal(err)
	}
	if err := authors.SetInternal([]byte("version"), []byte("2")); err != nil {
		t.Fatal(err)
	}
	if err := authors.Close(); err != nil {
		t.Fatal(err)
	}

	documents, authors, needsReindex := getIndexes(fs, 0.25, 0.1, 10000)
	idx := index.NewBleve(documents, authors, nil, "", nil, index.Config{})
	defer idx.Close()
	if needsReindex {
		t.Fatal("author migration must not require document reindexing")
	}
	if count, err := documents.DocCount(); err != nil || count != 1 {
		t.Fatalf("document index was not preserved: count=%d, err=%v", count, err)
	}
	stored, err := idx.Document("book")
	if err != nil {
		t.Fatal(err)
	}
	data, err := index.DataSourceData[biblioreads.DocumentData](stored, biblioreads.Source)
	if err != nil || !data.Enriched {
		t.Fatalf("author migration changed document enrichment state: %+v, %v", data, err)
	}
	author, err := idx.Author("alice-borchardt", "")
	if err != nil || author.DataSourceID != "Q1" || !author.RetrievedOn.Equal(retrieved) {
		t.Fatalf("expected existing author metadata preserved: %+v, %v", author, err)
	}
	if pending, err := idx.AuthorsWithoutInfo(); err != nil || len(pending) != 0 {
		t.Fatalf("index migrations queued enriched authors again: %+v, %v", pending, err)
	}
}
