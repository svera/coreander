package index_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
	datasourcemodel "github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/precisiondate"
)

type mockAuthorDataSource struct {
	byName        map[string]datasourcemodel.Author
	bySlug        map[string]datasourcemodel.Author
	entityIDs     map[string][]string
	calls         int
	searchCalls   int
	retrieveCalls int
	delay         time.Duration
	errName       string
}

func (m *mockAuthorDataSource) SearchAuthor(name string, _ []string) (datasourcemodel.Author, error) {
	m.calls++
	m.searchCalls++
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	if m.errName == name {
		return nil, errMockLookup
	}
	return m.byName[name], nil
}

var errMockLookup = errors.New("mock lookup failed")

func (m *mockAuthorDataSource) SearchEntityIDs(name string) ([]string, error) {
	m.calls++
	m.searchCalls++
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	if m.errName == name {
		return nil, errMockLookup
	}
	if m.entityIDs != nil {
		if ids, ok := m.entityIDs[name]; ok {
			return ids, nil
		}
	}
	if author, ok := m.byName[name]; ok && author != nil {
		return []string{author.SourceID()}, nil
	}
	return nil, nil
}

func (m *mockAuthorDataSource) RetrieveAuthor(_ []string, _ []string) (datasourcemodel.Author, error) {
	m.calls++
	m.retrieveCalls++
	return nil, nil
}

func (m *mockAuthorDataSource) RetrieveAuthors(candidates map[string][]string, _ []string, _ time.Duration, onResult func(string, datasourcemodel.Author) error) error {
	m.calls++
	m.retrieveCalls++
	for slug := range candidates {
		author := m.bySlug[slug]
		if err := onResult(slug, author); err != nil {
			return err
		}
	}
	return nil
}

type stubAuthor struct {
	sourceID string
}

func (a stubAuthor) BirthName() string                        { return "" }
func (a stubAuthor) Description(string) string                { return "A writer" }
func (a stubAuthor) InstanceOf() float64                      { return 1 }
func (a stubAuthor) Gender() float64                          { return 0 }
func (a stubAuthor) DateOfBirth() precisiondate.PrecisionDate { return precisiondate.PrecisionDate{} }
func (a stubAuthor) DateOfDeath() precisiondate.PrecisionDate { return precisiondate.PrecisionDate{} }
func (a stubAuthor) Image() string                            { return "" }
func (a stubAuthor) Website() string                          { return "" }
func (a stubAuthor) WikipediaLink(string) string              { return "" }
func (a stubAuthor) SourceID() string                         { return a.sourceID }
func (a stubAuthor) RetrievedOn() time.Time                   { return time.Now().UTC() }
func (a stubAuthor) Pseudonyms() []string                     { return nil }

func TestAuthorsWithoutInfo(t *testing.T) {
	authorsIndexMem, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	documentsIndexMem, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}

	idx := index.NewBleve(documentsIndexMem, authorsIndexMem, nil, "", nil, index.Config{})

	if err := idx.IndexAuthor(index.Author{Slug: "enriched", Name: "Enriched Author", RetrievedOn: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexAuthor(index.Author{Slug: "pending", Name: "Pending Author"}); err != nil {
		t.Fatal(err)
	}

	authors, err := idx.AuthorsWithoutInfo()
	if err != nil {
		t.Fatal(err)
	}
	if len(authors) != 1 {
		t.Fatalf("expected 1 author without info, got %d", len(authors))
	}
	if authors[0].Slug != "pending" {
		t.Fatalf("expected pending author, got %q", authors[0].Slug)
	}
}

func TestEnrichAuthorsFromDataSource(t *testing.T) {
	authorsIndexMem, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	documentsIndexMem, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}

	idx := index.NewBleve(documentsIndexMem, authorsIndexMem, nil, "", nil, index.Config{})

	if err := idx.IndexAuthor(index.Author{Slug: "found", Name: "Found Author"}); err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexAuthor(index.Author{Slug: "missing", Name: "Missing Author"}); err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexAuthor(index.Author{Slug: "done", Name: "Done Author", RetrievedOn: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	dataSource := &mockAuthorDataSource{
		bySlug: map[string]datasourcemodel.Author{
			"found": stubAuthor{sourceID: "Q1"},
		},
		entityIDs: map[string][]string{
			"Found Author": {"Q1"},
		},
	}

	start := time.Now()
	if err := idx.EnrichAuthorsFromDataSource(dataSource, []string{"en"}, 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	if dataSource.searchCalls != 2 {
		t.Fatalf("expected 2 Wikidata name searches, got %d", dataSource.searchCalls)
	}
	if dataSource.retrieveCalls != 1 {
		t.Fatalf("expected 1 batched Wikidata entity fetch, got %d", dataSource.retrieveCalls)
	}
	if elapsed < 50*time.Millisecond {
		t.Fatalf("expected throttling between lookups, took %s", elapsed)
	}

	found, err := idx.Author("found", "en")
	if err != nil {
		t.Fatal(err)
	}
	if found.DataSourceID != "Q1" {
		t.Fatalf("expected enriched author Q1, got %q", found.DataSourceID)
	}

	missing, err := idx.Author("missing", "en")
	if err != nil {
		t.Fatal(err)
	}
	if missing.RetrievedOn.IsZero() {
		t.Fatal("expected missing author to be marked as processed")
	}
	if missing.DataSourceID != "" {
		t.Fatalf("expected missing author to have no data source id, got %q", missing.DataSourceID)
	}

	remaining, err := idx.AuthorsWithoutInfo()
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected no authors left to enrich, got %d", len(remaining))
	}
}

func TestAuthorEnrichmentProgress(t *testing.T) {
	authorsIndexMem, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	documentsIndexMem, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}

	idx := index.NewBleve(documentsIndexMem, authorsIndexMem, nil, "", nil, index.Config{})

	if err := idx.IndexAuthor(index.Author{Slug: "one", Name: "Author One"}); err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexAuthor(index.Author{Slug: "two", Name: "Author Two"}); err != nil {
		t.Fatal(err)
	}

	dataSource := &mockAuthorDataSource{delay: 40 * time.Millisecond}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = idx.EnrichAuthorsFromDataSource(dataSource, []string{"en"}, time.Millisecond)
	}()

	deadline := time.Now().Add(2 * time.Second)
	sawProgress := false
	for time.Now().Before(deadline) {
		p, err := idx.IndexingProgress()
		if err != nil {
			t.Fatal(err)
		}
		if p.InProgress && p.Percentage > 0 && p.Kind == index.ProgressAuthors {
			sawProgress = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	wg.Wait()

	if !sawProgress {
		t.Fatal("expected IndexingProgress to report author enrichment percentage > 0")
	}
}
