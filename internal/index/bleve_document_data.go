package index

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/blevesearch/bleve/v2"
)

// AuthorLink is a provider-owned link discovered while enriching a document.
type AuthorLink struct {
	Slug   string
	Name   string
	Source string
	URL    string
}

// DocumentUpdate groups document changes and discovered author links.
type DocumentUpdate struct {
	Document
	AuthorLinks []AuthorLink
}

// DocumentByID retrieves a document using its index ID rather than its public slug.
func (b *BleveIndexer) DocumentByID(id string) (Document, error) {
	return b.documentByIndexID(id)
}

// UpdateDocument applies changes to the latest stored document under the write lock.
// The callback returns false when no write is needed. It must not call other index methods.
func (b *BleveIndexer) UpdateDocument(id string, update func(*DocumentUpdate) (bool, error)) error {
	b.documentsMu.Lock()
	defer b.documentsMu.Unlock()
	document, found, err := b.documentByIndexIDLocked(id)
	if err != nil {
		return fmt.Errorf("reading document %s for update: %w", id, err)
	}
	if !found {
		return ErrDocumentNotFound
	}
	pending := DocumentUpdate{Document: document}
	changed, err := update(&pending)
	if err != nil || !changed {
		return err
	}
	for _, link := range pending.AuthorLinks {
		if err := b.storeAuthorLinkLocked(link); err != nil {
			return err
		}
	}
	return IndexDocument(b.documentsIdx, id, pending.Document)
}

// storeAuthorLinkLocked requires documentsMu to be held.
func (b *BleveIndexer) storeAuthorLinkLocked(link AuthorLink) error {
	if link.Slug == "" || link.Source == "" || link.URL == "" {
		return fmt.Errorf("author link must have a slug, source and URL")
	}
	b.authorsMu.Lock()
	defer b.authorsMu.Unlock()
	author, err := b.authorBySlugLocked(link.Slug)
	if err != nil {
		return err
	}
	if author.Slug == "" {
		author = Author{Slug: link.Slug, Name: link.Name}
		for _, field := range []string{"AuthorsSlugs", "IllustratorsSlugs"} {
			q := bleve.NewTermQuery(link.Slug)
			q.SetField(field)
			result, err := b.documentsIdx.Search(bleve.NewSearchRequestOptions(q, 0, 0, false))
			if err != nil {
				return fmt.Errorf("counting documents for author %s: %w", link.Slug, err)
			}
			author.DocumentCount += result.Total
		}
	}
	if author.DataSourceLinks[link.Source] != "" {
		return nil
	}
	if author.DataSourceLinks == nil {
		author.DataSourceLinks = make(map[string]string)
	}
	author.DataSourceLinks[link.Source] = link.URL
	return indexAuthor(b.authorsIdx, link.Slug, author)
}

func (b *BleveIndexer) documentByIndexIDLocked(id string) (Document, bool, error) {
	req := bleve.NewSearchRequest(bleve.NewDocIDQuery([]string{id}))
	req.Fields = []string{"*"}
	result, err := b.documentsIdx.Search(req)
	if err != nil || len(result.Hits) == 0 {
		return Document{}, false, err
	}
	document := hydrateDocument(result.Hits[0])
	document.DataSources, err = decodeDataSources(result.Hits[0].Fields, id)
	return document, true, err
}

// preserveDataSourcesLocked protects provider updates from stale metadata writers.
// It requires documentsMu to be held for writing.
func (b *BleveIndexer) preserveDataSourcesLocked(documents []Document) error {
	if len(documents) == 0 {
		return nil
	}
	ids := make([]string, len(documents))
	for i, document := range documents {
		ids[i] = document.ID
	}
	req := bleve.NewSearchRequestOptions(bleve.NewDocIDQuery(ids), len(ids), 0, false)
	req.Fields = []string{"DataSources", "DataSourceScoreValue"}
	result, err := b.documentsIdx.Search(req)
	if err != nil {
		return fmt.Errorf("reading stored data source fields: %w", err)
	}
	stored := make(map[string]Document, len(result.Hits))
	for _, hit := range result.Hits {
		ratingValue, _ := hit.Fields["DataSourceScoreValue"].(float64)
		sources, err := decodeDataSources(hit.Fields, hit.ID)
		if err != nil {
			return err
		}
		stored[hit.ID] = Document{DataSources: sources, DataSourceScoreValue: ratingValue}
	}
	for i := range documents {
		if current, ok := stored[documents[i].ID]; ok {
			merged := maps.Clone(documents[i].DataSources)
			if merged == nil && len(current.DataSources) > 0 {
				merged = make(map[string]json.RawMessage)
			}
			maps.Copy(merged, current.DataSources)
			documents[i].DataSources = merged
			documents[i].DataSourceScoreValue = current.DataSourceScoreValue
		}
	}
	return nil
}
