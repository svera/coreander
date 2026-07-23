package index

import (
	"log"
	"time"

	"github.com/blevesearch/bleve/v2"
	datasourcemodel "github.com/svera/coreander/v5/internal/datasource/model"
)

func (b *BleveIndexer) incrementAuthorCounts(names, slugs []string) error {
	for i, name := range names {
		if i < len(slugs) {
			if err := b.incrementAuthorCount(name, slugs[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// incrementAuthorCount creates the author with DocumentCount=1 if not yet indexed,
// or increments their DocumentCount if they already exist.
func (b *BleveIndexer) incrementAuthorCount(name, authorSlug string) error {
	if name == "" || authorSlug == "" {
		return nil
	}
	existing, err := b.Author(authorSlug, "")
	if err != nil {
		return err
	}
	if existing.Slug == "" {
		existing = Author{Name: name, Slug: authorSlug, DocumentCount: 1}
	} else {
		existing.DocumentCount++
	}
	return b.authorsIdx.Index(authorSlug, existing)
}

// RebuildAuthorsFromDocuments recalculates DocumentCount for every author from the documents
// index, creating missing author entries and updating existing ones.
func (b *BleveIndexer) RebuildAuthorsFromDocuments(batchSize int) error {
	counts, names, err := b.countDocumentsPerAuthor()
	if err != nil {
		return err
	}
	if len(counts) == 0 {
		return nil
	}

	// Fetch all existing authors so their enriched metadata is preserved.
	authorDocCount, err := b.authorsIdx.DocCount()
	if err != nil {
		return err
	}
	existingAuthors := make(map[string]Author, authorDocCount)
	if authorDocCount > 0 {
		req := bleve.NewSearchRequestOptions(bleve.NewMatchAllQuery(), int(authorDocCount), 0, false)
		req.Fields = []string{"*"}
		result, err := b.authorsIdx.Search(req)
		if err != nil {
			return err
		}
		for _, hit := range result.Hits {
			a := hydrateAuthor(hit)
			existingAuthors[a.Slug] = a
		}
	}

	batch := b.authorsIdx.NewBatch()
	for authorSlug, count := range counts {
		author, exists := existingAuthors[authorSlug]
		if !exists {
			author = Author{Name: names[authorSlug], Slug: authorSlug}
		}
		author.DocumentCount = count
		if err := batch.Index(authorSlug, author); err != nil {
			return err
		}
		if batch.Size() >= batchSize {
			if err := b.authorsIdx.Batch(batch); err != nil {
				return err
			}
			batch.Reset()
		}
	}
	if batch.Size() > 0 {
		return b.authorsIdx.Batch(batch)
	}
	return nil
}

// countDocumentsPerAuthor scans the documents index and returns per-author document counts
// and one representative name per author slug.
func (b *BleveIndexer) countDocumentsPerAuthor() (counts map[string]uint64, names map[string]string, err error) {
	docCount, err := b.documentsIdx.DocCount()
	if err != nil {
		return nil, nil, err
	}
	if docCount == 0 {
		return map[string]uint64{}, map[string]string{}, nil
	}

	req := bleve.NewSearchRequestOptions(bleve.NewMatchAllQuery(), int(docCount), 0, false)
	req.Fields = []string{"*"}
	result, err := b.documentsIdx.Search(req)
	if err != nil {
		return nil, nil, err
	}

	counts = make(map[string]uint64)
	names = make(map[string]string)
	for _, hit := range result.Hits {
		document := hydrateDocument(hit)
		accumulateContributors(counts, names, document.AuthorsSlugs, document.Authors)
		accumulateContributors(counts, names, document.IllustratorsSlugs, document.Illustrators)
	}
	return counts, names, nil
}

func accumulateContributors(counts map[string]uint64, names map[string]string, slugs []string, displayNames []string) {
	for i, slug := range slugs {
		if slug == "" {
			continue
		}
		counts[slug]++
		if _, seen := names[slug]; !seen && i < len(displayNames) {
			names[slug] = displayNames[i]
		}
	}
}

func (b *BleveIndexer) IndexAuthor(author Author) error {
	if err := b.authorsIdx.Index(author.Slug, author); err != nil {
		return err
	}
	return nil
}

func (b *BleveIndexer) beginAuthorEnrichment(total int) {
	b.authorEnrichStartNanos.Store(time.Now().UnixNano())
	b.authorEnrichProcessed.Store(0)
	b.authorEnrichTotalEntries.Store(uint64(total))
}

func (b *BleveIndexer) endAuthorEnrichment() {
	b.authorEnrichStartNanos.Store(0)
	b.authorEnrichProcessed.Store(0)
	b.authorEnrichTotalEntries.Store(0)
}

func (b *BleveIndexer) recordAuthorEnrichmentProgress() {
	b.authorEnrichProcessed.Add(1)
}

const (
	AuthorEnrichRequestsPerMinute = 200
	DefaultAuthorEnrichInterval   = time.Minute / AuthorEnrichRequestsPerMinute
)

// AuthorDataSource retrieves author metadata from an external source such as Wikidata.
type AuthorDataSource interface {
	SearchAuthor(name string, languages []string) (datasourcemodel.Author, error)
	SearchEntityIDs(name string) ([]string, error)
	RetrieveAuthor(ids []string, languages []string) (datasourcemodel.Author, error)
	RetrieveAuthors(candidates map[string][]string, languages []string, batchInterval time.Duration, onResult func(slug string, author datasourcemodel.Author) error) error
}

// CombineWithDataSource merges external author metadata into an indexed author.
func CombineWithDataSource(author *Author, authorDataSource datasourcemodel.Author, supportedLanguages []string) {
	author.DataSourceID = authorDataSource.SourceID()
	author.BirthName = authorDataSource.BirthName()
	author.RetrievedOn = authorDataSource.RetrievedOn()
	author.WikipediaLink = make(map[string]string)
	author.InstanceOf = authorDataSource.InstanceOf()
	author.Description = make(map[string]string)
	author.DateOfBirth = authorDataSource.DateOfBirth()
	author.DateOfDeath = authorDataSource.DateOfDeath()
	author.Website = authorDataSource.Website()
	author.DataSourceImage = authorDataSource.Image()
	author.Gender = authorDataSource.Gender()
	author.Pseudonyms = make([]string, 0, len(authorDataSource.Pseudonyms()))

	for _, pseudonym := range authorDataSource.Pseudonyms() {
		if pseudonym != author.Name {
			author.Pseudonyms = append(author.Pseudonyms, pseudonym)
		}
	}

	for _, lang := range supportedLanguages {
		author.WikipediaLink[lang] = authorDataSource.WikipediaLink(lang)
		author.Description[lang] = authorDataSource.Description(lang)
	}
}

// EnrichAuthorsFromDataSource fetches metadata for authors missing external info and updates the index.
// Name searches are throttled to at most one lookup per interval; entity details are fetched in batches.
func (b *BleveIndexer) EnrichAuthorsFromDataSource(dataSource AuthorDataSource, supportedLanguages []string, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultAuthorEnrichInterval
	}

	authors, err := b.AuthorsWithoutInfo()
	if err != nil {
		return err
	}
	if len(authors) == 0 {
		return nil
	}

	log.Printf("Enriching %d authors from Wikidata", len(authors))

	b.beginAuthorEnrichment(len(authors) * 2)
	defer b.endAuthorEnrichment()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	candidates := make(map[string][]string, len(authors))
	authorsBySlug := make(map[string]Author, len(authors))

	searchIndex := 0
	for _, author := range authors {
		authorsBySlug[author.Slug] = author
		if author.DataSourceID != "" {
			candidates[author.Slug] = []string{author.DataSourceID}
			b.recordAuthorEnrichmentProgress()
			continue
		}

		if searchIndex > 0 {
			<-ticker.C
		}
		searchIndex++

		ids, err := dataSource.SearchEntityIDs(author.Name)
		if err != nil {
			log.Printf("Error searching author %s on Wikidata: %s", author.Name, err)
			candidates[author.Slug] = nil
		} else {
			candidates[author.Slug] = ids
		}
		b.recordAuthorEnrichmentProgress()
	}

	err = dataSource.RetrieveAuthors(candidates, supportedLanguages, interval, func(slug string, authorData datasourcemodel.Author) error {
		author := authorsBySlug[slug]
		if authorData != nil {
			CombineWithDataSource(&author, authorData, supportedLanguages)
		} else {
			author.RetrievedOn = time.Now().UTC()
		}
		if err := b.IndexAuthor(author); err != nil {
			log.Printf("Error indexing enriched author %s: %s", author.Name, err)
		}
		b.recordAuthorEnrichmentProgress()
		return nil
	})

	log.Printf("Author enrichment finished")
	return err
}
