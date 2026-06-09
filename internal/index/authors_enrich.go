package index

import (
	"log"
	"time"

	"github.com/blevesearch/bleve/v2"
	datasourcemodel "github.com/svera/coreander/v4/internal/datasource/model"
)

const (
	AuthorEnrichRequestsPerMinute = 200
	DefaultAuthorEnrichInterval   = time.Minute / AuthorEnrichRequestsPerMinute
)

// AuthorDataSource retrieves author metadata from an external source such as Wikidata.
type AuthorDataSource interface {
	SearchAuthor(name string, languages []string) (datasourcemodel.Author, error)
	RetrieveAuthor(ids []string, languages []string) (datasourcemodel.Author, error)
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

// AuthorsWithoutInfo returns indexed authors that have not been enriched from an external source yet.
func (b *BleveIndexer) AuthorsWithoutInfo() ([]Author, error) {
	count, err := b.authorsIdx.DocCount()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}

	searchReq := bleve.NewSearchRequest(bleve.NewMatchAllQuery())
	searchReq.Fields = []string{"*"}
	searchReq.Size = int(count)

	searchResult, err := b.authorsIdx.Search(searchReq)
	if err != nil {
		return nil, err
	}

	authors := make([]Author, 0, len(searchResult.Hits))
	for _, hit := range searchResult.Hits {
		author := hydrateAuthor(hit)
		if author.RetrievedOn.IsZero() {
			authors = append(authors, author)
		}
	}
	return authors, nil
}

// EnrichAuthorsFromDataSource fetches metadata for authors missing external info and updates the index.
// Requests are throttled to at most one author lookup per interval.
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

	b.beginAuthorEnrichment(len(authors))
	defer b.endAuthorEnrichment()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for i, author := range authors {
		if i > 0 {
			<-ticker.C
		}

		authorDataSource, err := b.fetchAuthorFromDataSource(dataSource, author, supportedLanguages)
		if err != nil {
			log.Printf("Error retrieving author %s from Wikidata: %s", author.Name, err)
			b.recordAuthorEnrichmentProgress()
			continue
		}

		if authorDataSource == nil {
			author.RetrievedOn = time.Now().UTC()
		} else {
			CombineWithDataSource(&author, authorDataSource, supportedLanguages)
		}

		if err := b.IndexAuthor(author); err != nil {
			log.Printf("Error indexing enriched author %s: %s", author.Name, err)
		}
		b.recordAuthorEnrichmentProgress()
	}

	log.Printf("Author enrichment finished")
	return nil
}

func (b *BleveIndexer) fetchAuthorFromDataSource(dataSource AuthorDataSource, author Author, supportedLanguages []string) (datasourcemodel.Author, error) {
	if author.DataSourceID != "" {
		return dataSource.RetrieveAuthor([]string{author.DataSourceID}, supportedLanguages)
	}
	return dataSource.SearchAuthor(author.Name, supportedLanguages)
}
