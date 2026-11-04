package biblioreads

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/svera/coreander/v5/internal/index"
)

const (
	// maxConcurrentLookups bounds lookups across every enrichment caller, so they do not pile up
	// waiting on the client. Client request spacing and cooldowns protect each instance.
	maxConcurrentLookups = 4
	requestTimeout       = 2 * time.Minute
)

type SearchSource interface {
	SearchBooks(context.Context, string) (SearchBooksResponse, error)
}

type Enricher struct {
	source      SearchSource
	idx         *index.BleveIndexer
	lookupSlots chan struct{}
}

// NewEnricher adapts a  source to the document-enrichment pipeline.
func NewEnricher(source SearchSource) *Enricher {
	return &Enricher{source: source, lookupSlots: make(chan struct{}, maxConcurrentLookups)}
}

// SetIndex binds the adapter before starting background or on-demand enrichment.
func (p *Enricher) SetIndex(idx *index.BleveIndexer) {
	p.idx = idx
}

func (*Enricher) Source() string { return Source }

func (*Enricher) Name() string { return Name }

func (*Enricher) NeedsEnrichment(document index.Document) (bool, error) {
	data, err := index.DataSourceData[DocumentData](document, Source)
	return !data.Enriched, err
}

func (p *Enricher) Enrich(ctx context.Context, idx *index.BleveIndexer, document index.Document) error {
	_, err := p.enrich(ctx, idx, document)
	return err
}

// Book returns a stored match or performs an on-demand lookup.
func (p *Enricher) Book(ctx context.Context, document index.Document) (SearchResult, error) {
	data, err := index.DataSourceData[DocumentData](document, Source)
	if err != nil {
		return SearchResult{}, err
	}
	if data.URL != "" || data.Enriched || p.source == nil {
		return storedResult(document, data), nil
	}
	return p.enrich(ctx, p.idx, document)
}

func storedResult(document index.Document, data DocumentData) SearchResult {
	return SearchResult{
		BookURL: data.URL, Rating: fmt.Sprintf("%.2f", document.DataSourceScoreValue), RatingCount: data.RatingCount,
	}
}

// UpdateDataSourceLink clears cached  data when an administrator corrects a match.
func (p *Enricher) UpdateDataSourceLink(documentID, reviewLink string) error {
	if p.idx == nil {
		return fmt.Errorf("BiblioReads adapter has no index")
	}
	return p.idx.UpdateDocument(documentID, func(current *index.DocumentUpdate) (bool, error) {
		data, err := index.DataSourceData[DocumentData](current.Document, Source)
		if err != nil || data.URL == reviewLink {
			return false, err
		}
		current.DataSourceScoreValue = 0
		err = index.SetDataSourceData(&current.Document, Source, DocumentData{URL: reviewLink, Enriched: true})
		return true, err
	})
}

// StoreDetails rejects responses for a match or metadata changed during the fetch.
func (p *Enricher) StoreDetails(document index.Document, bookURL string, book BookResponse) error {
	if p.idx == nil {
		return fmt.Errorf("BiblioReads adapter has no index")
	}
	if bookURL == "" {
		return fmt.Errorf("invalid Goodreads book URL for document %s", document.ID)
	}
	hasIDs := book.ResourceID != "" && book.LegacyBookID > 0
	if book.Rating == "" && book.RatingCount == "" && !hasIDs {
		return nil
	}
	return p.idx.UpdateDocument(document.ID, func(current *index.DocumentUpdate) (bool, error) {
		data, err := index.DataSourceData[DocumentData](current.Document, Source)
		if err != nil {
			return false, err
		}
		if searchQuery(current.Document) != searchQuery(document) || data.URL != bookURL {
			return false, fmt.Errorf("BiblioReads match changed for document %s", document.ID)
		}
		changed := false
		if book.RatingCount != "" && data.RatingCount != book.RatingCount {
			data.RatingCount = book.RatingCount
			changed = true
		}
		if book.Rating != "" {
			value := parseRating(book.Rating, document.ID)
			if current.DataSourceScoreValue != value {
				current.DataSourceScoreValue = value
				changed = true
			}
		}
		if hasIDs && (data.ResourceID != book.ResourceID || data.LegacyBookID != book.LegacyBookID) {
			data.ResourceID, data.LegacyBookID = book.ResourceID, book.LegacyBookID
			changed = true
		}
		if !changed {
			return false, nil
		}
		return true, index.SetDataSourceData(&current.Document, Source, data)
	})
}

func (p *Enricher) enrich(ctx context.Context, idx *index.BleveIndexer, document index.Document) (SearchResult, error) {
	if idx == nil || p.source == nil {
		return SearchResult{}, fmt.Errorf("BiblioReads enrichment requires a source and index")
	}
	data, err := index.DataSourceData[DocumentData](document, Source)
	if err != nil {
		return SearchResult{}, err
	}
	select {
	case p.lookupSlots <- struct{}{}:
		defer func() { <-p.lookupSlots }()
	case <-ctx.Done():
		return SearchResult{}, ctx.Err()
	}
	q := searchQuery(document)
	result := SearchResult{BookURL: data.URL}
	if result.BookURL == "" && document.Title != "" {
		ctx, cancel := context.WithTimeout(ctx, requestTimeout)
		defer cancel()
		queries := []string{q}
		if title := strings.TrimSpace(document.Title); title != q {
			queries = append(queries, title)
		}
		for _, query := range queries {
			response, err := p.source.SearchBooks(ctx, query)
			if err != nil {
				return SearchResult{}, fmt.Errorf("error searching %q on BiblioReads: %w", query, err)
			}
			if match, ok := matchingBook(document, response.Result); ok {
				result = match
			}
			if result.BookURL != "" {
				break
			}
		}
	}
	err = idx.UpdateDocument(document.ID, func(current *index.DocumentUpdate) (bool, error) {
		if searchQuery(current.Document) != q {
			return false, nil
		}
		data, err := index.DataSourceData[DocumentData](current.Document, Source)
		if err != nil {
			return false, err
		}
		if data.Enriched {
			result = storedResult(current.Document, data)
			return false, nil
		}
		if data.URL != "" && data.URL != result.BookURL {
			result = storedResult(current.Document, data)
		}
		data.URL, data.RatingCount, data.Enriched = result.BookURL, result.RatingCount, true
		if result.Rating == "" && current.DataSourceScoreValue > 0 {
			result.Rating = fmt.Sprintf("%.2f", current.DataSourceScoreValue)
		}
		current.DataSourceScoreValue = parseRating(result.Rating, current.ID)
		if result.AuthorURL != "" {
			if slug, name := matchedAuthor(current.Document, result.Author); slug != "" {
				current.AuthorLinks = append(current.AuthorLinks, index.AuthorLink{
					Slug: slug, Name: name, Source: Source, URL: result.AuthorURL,
				})
			}
		}
		return true, index.SetDataSourceData(&current.Document, Source, data)
	})
	if errors.Is(err, index.ErrDocumentNotFound) {
		return result, nil
	}
	return result, err
}
