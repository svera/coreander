// Package goodreads adapts Goodreads matching and review storage to the generic index.
package goodreads

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/index"
)

const (
	goodReadsEnrichWorkers  = 4
	goodReadsRequestTimeout = 2 * time.Minute
)

type GoodReadsSource interface {
	SearchBooks(context.Context, string) (biblioreads.SearchBooksResponse, error)
}

type Enricher struct {
	source GoodReadsSource
	idx    *index.BleveIndexer
	slots  chan struct{}
}

func New(source GoodReadsSource) *Enricher {
	return &Enricher{source: source, slots: make(chan struct{}, goodReadsEnrichWorkers)}
}

// SetIndex binds the adapter before starting background or on-demand enrichment.
func (p *Enricher) SetIndex(idx *index.BleveIndexer) {
	p.idx = idx
}

func (*Enricher) Source() string { return biblioreads.Source }

func (*Enricher) NeedsEnrichment(document index.Document) (bool, error) {
	data, err := index.DataSourceData[biblioreads.DocumentData](document, biblioreads.Source)
	return !data.Enriched, err
}

func (p *Enricher) Enrich(ctx context.Context, idx *index.BleveIndexer, document index.Document) error {
	_, err := p.enrich(ctx, idx, document)
	return err
}

// GoodReadsBook returns a stored match or performs an on-demand lookup.
func (p *Enricher) GoodReadsBook(ctx context.Context, document index.Document) (biblioreads.SearchResult, error) {
	data, err := index.DataSourceData[biblioreads.DocumentData](document, biblioreads.Source)
	if err != nil {
		return biblioreads.SearchResult{}, err
	}
	if data.URL != "" || data.Enriched || p.source == nil {
		return storedResult(document, data), nil
	}
	return p.enrich(ctx, p.idx, document)
}

func storedResult(document index.Document, data biblioreads.DocumentData) biblioreads.SearchResult {
	return biblioreads.SearchResult{
		BookURL: data.URL, Rating: fmt.Sprintf("%.2f", document.DataSourceScoreValue), RatingCount: data.RatingCount,
	}
}

// UpdateDataSourceLink clears cached Goodreads data when an administrator corrects a match.
func (p *Enricher) UpdateDataSourceLink(documentID, reviewLink string) error {
	if p.idx == nil {
		return fmt.Errorf("Goodreads adapter has no index")
	}
	return p.idx.UpdateDocument(documentID, func(current *index.DocumentUpdate) (bool, error) {
		data, err := index.DataSourceData[biblioreads.DocumentData](current.Document, biblioreads.Source)
		if err != nil || data.URL == reviewLink {
			return false, err
		}
		current.DataSourceScoreValue = 0
		err = index.SetDataSourceData(&current.Document, biblioreads.Source, biblioreads.DocumentData{URL: reviewLink, Enriched: true})
		return true, err
	})
}

// StoreGoodReadsDetails rejects responses for a match or metadata changed during the fetch.
func (p *Enricher) StoreGoodReadsDetails(document index.Document, bookURL string, book biblioreads.BookResponse) error {
	if p.idx == nil {
		return fmt.Errorf("Goodreads adapter has no index")
	}
	if bookURL == "" {
		return fmt.Errorf("invalid GoodReads book URL for document %s", document.ID)
	}
	hasIDs := book.ResourceID != "" && book.LegacyBookID > 0
	if book.Rating == "" && book.RatingCount == "" && !hasIDs {
		return nil
	}
	return p.idx.UpdateDocument(document.ID, func(current *index.DocumentUpdate) (bool, error) {
		data, err := index.DataSourceData[biblioreads.DocumentData](current.Document, biblioreads.Source)
		if err != nil {
			return false, err
		}
		if goodReadsQuery(current.Document) != goodReadsQuery(document) || data.URL != bookURL {
			return false, fmt.Errorf("GoodReads match changed for document %s", document.ID)
		}
		changed := false
		if book.RatingCount != "" && data.RatingCount != book.RatingCount {
			data.RatingCount = book.RatingCount
			changed = true
		}
		if book.Rating != "" {
			value := parseGoodReadsRating(book.Rating, document.ID)
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
		return true, index.SetDataSourceData(&current.Document, biblioreads.Source, data)
	})
}

func (p *Enricher) enrich(ctx context.Context, idx *index.BleveIndexer, document index.Document) (biblioreads.SearchResult, error) {
	if idx == nil || p.source == nil {
		return biblioreads.SearchResult{}, fmt.Errorf("Goodreads enrichment requires a source and index")
	}
	data, err := index.DataSourceData[biblioreads.DocumentData](document, biblioreads.Source)
	if err != nil {
		return biblioreads.SearchResult{}, err
	}
	select {
	case p.slots <- struct{}{}:
		defer func() { <-p.slots }()
	case <-ctx.Done():
		return biblioreads.SearchResult{}, ctx.Err()
	}
	q := goodReadsQuery(document)
	result := biblioreads.SearchResult{BookURL: data.URL}
	if result.BookURL == "" && document.Title != "" {
		ctx, cancel := context.WithTimeout(ctx, goodReadsRequestTimeout)
		defer cancel()
		queries := []string{q}
		if title := strings.TrimSpace(document.Title); title != q {
			queries = append(queries, title)
		}
		for _, query := range queries {
			response, err := p.source.SearchBooks(ctx, query)
			switch {
			case errors.Is(err, biblioreads.ErrUnavailable), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
				return biblioreads.SearchResult{}, fmt.Errorf("error searching %q on GoodReads: %w", query, err)
			case err != nil:
				log.Printf("Error searching %q on GoodReads: %s", query, err)
			default:
				if match, ok := matchingGoodReadsBook(document, response.Result); ok {
					result = match
				}
			}
			if result.BookURL != "" {
				break
			}
		}
	}
	err = idx.UpdateDocument(document.ID, func(current *index.DocumentUpdate) (bool, error) {
		if goodReadsQuery(current.Document) != q {
			return false, nil
		}
		data, err := index.DataSourceData[biblioreads.DocumentData](current.Document, biblioreads.Source)
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
		current.DataSourceScoreValue = parseGoodReadsRating(result.Rating, current.ID)
		if result.AuthorURL != "" {
			if slug, name := goodReadsAuthor(current.Document, result.Author); slug != "" {
				current.AuthorLinks = append(current.AuthorLinks, index.AuthorLink{
					Slug: slug, Name: name, Source: biblioreads.Source, URL: result.AuthorURL,
				})
			}
		}
		return true, index.SetDataSourceData(&current.Document, biblioreads.Source, data)
	})
	if errors.Is(err, index.ErrDocumentNotFound) {
		return result, nil
	}
	return result, err
}
