package reviews

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/servicelinks"
)

const (
	goodReadsBaseURL    = "https://www.goodreads.com"
	goodReadsBookPrefix = "/book/show/"
)

type BiblioReadsClient interface {
	Book(ctx context.Context, id string) (biblioreads.BookResponse, error)
	Reviews(ctx context.Context, resourceID string, legacyBookID int, nextPageToken, languageCode string) (biblioreads.ReviewsResponse, error)
}

type GoodReadsIndex interface {
	GoodReadsBook(ctx context.Context, document index.Document) (biblioreads.SearchResult, error)
	StoreGoodReadsBookIDs(document index.Document, bookURL, resourceID string, legacyBookID int) error
}

type BiblioReads struct {
	client BiblioReadsClient
	index  GoodReadsIndex
}

func NewBiblioReads(client BiblioReadsClient, idx GoodReadsIndex) *BiblioReads {
	return &BiblioReads{client: client, index: idx}
}

func bookID(bookURL string) (string, error) {
	id, ok := strings.CutPrefix(bookURL, goodReadsBookPrefix)
	if !ok || id == "" {
		return "", fmt.Errorf("invalid Goodreads book URL %q", bookURL)
	}
	return id, nil
}

func (b *BiblioReads) book(ctx context.Context, document index.Document) (biblioreads.BookResponse, error) {
	id, err := bookID(document.GoodReadsBookURL)
	if err != nil {
		return biblioreads.BookResponse{}, err
	}
	book, err := b.client.Book(ctx, id)
	if err != nil {
		return book, fmt.Errorf("retrieving Goodreads book %s: %w", id, err)
	}
	if book.ResourceID != "" && book.LegacyBookID > 0 {
		if err := b.index.StoreGoodReadsBookIDs(document, document.GoodReadsBookURL, book.ResourceID, book.LegacyBookID); err != nil {
			log.Printf("Error saving Goodreads identifiers for %s: %s", document.ID, err)
		}
	}
	return book, nil
}

func (b *BiblioReads) Score(ctx context.Context, document index.Document) (Score, error) {
	score := Score{Source: "Goodreads"}
	if document.GoodReadsBookURL == "" {
		result, err := b.index.GoodReadsBook(ctx, document)
		if err != nil {
			return score, err
		}
		if result.BookURL == "" {
			return score, nil
		}
		if _, err := bookID(result.BookURL); err != nil {
			return score, err
		}
		document.GoodReadsBookURL = result.BookURL
		score.Value, score.URL = result.Rating, (servicelinks.Goodreads{}).Document(document).URL
		return score, nil
	}
	book, err := b.book(ctx, document)
	if err != nil {
		return score, err
	}
	score.Value, score.Count, score.URL = book.Rating, book.RatingCount, (servicelinks.Goodreads{}).Document(document).URL
	return score, nil
}

func (b *BiblioReads) Reviews(ctx context.Context, document index.Document, language string) (Page, error) {
	if document.GoodReadsBookURL == "" {
		return Page{}, nil
	}
	book, err := b.book(ctx, document)
	if err != nil {
		return Page{}, err
	}
	if book.Rating == "" {
		return Page{}, nil
	}
	resourceID, legacyBookID := document.GoodReadsResourceID, document.GoodReadsLegacyBookID
	if book.ResourceID != "" && book.LegacyBookID > 0 {
		resourceID, legacyBookID = book.ResourceID, book.LegacyBookID
	}
	if resourceID == "" || legacyBookID <= 0 {
		return Page{}, fmt.Errorf("missing Goodreads review identifiers for %s", document.ID)
	}
	response, err := b.client.Reviews(ctx, resourceID, legacyBookID, "", language)
	if err != nil {
		return Page{}, fmt.Errorf("retrieving Goodreads reviews for %s: %w", document.ID, err)
	}
	page := Page{Source: "Goodreads", URL: (servicelinks.Goodreads{}).Document(document).URL, Available: true}
	for _, review := range response.Reviews {
		var createdAt time.Time
		if review.CreatedAt != "" {
			createdAt, err = time.Parse("Mon Jan 02 2006", review.CreatedAt)
			if err != nil {
				log.Printf("Invalid Goodreads review date for %s: %q: %s", review.ID, review.CreatedAt, err)
			}
		}
		reviewURL := ""
		if review.ReviewURL != "" {
			reviewURL = goodReadsBaseURL + review.ReviewURL
		}
		page.Reviews = append(page.Reviews, Review{
			Reviewer: review.Reviewer.Name, Rating: review.Rating, Text: review.Text,
			CreatedAt: createdAt, Spoiler: review.Spoiler, URL: reviewURL,
		})
	}
	return page, nil
}
