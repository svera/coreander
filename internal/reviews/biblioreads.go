package reviews

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	datasourcemodel "github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/index"
)

const (
	goodReadsBaseURL    = "https://www.goodreads.com"
	goodReadsBookPrefix = "/book/show/"
)

var goodReadsBookPath = regexp.MustCompile(`^/book/show/([1-9][0-9]*)(?:[.-][A-Za-z0-9._~-]+)?/?$`)

type BiblioReadsClient interface {
	Book(ctx context.Context, id string) (biblioreads.BookResponse, error)
	Reviews(ctx context.Context, resourceID string, legacyBookID int, nextPageToken, languageCode string) (biblioreads.ReviewsResponse, error)
}

type GoodReadsIndex interface {
	GoodReadsBook(ctx context.Context, document index.Document) (biblioreads.SearchResult, error)
	StoreGoodReadsDetails(document index.Document, bookURL string, book biblioreads.BookResponse) error
	UpdateDataSourceLink(documentID, bookURL string) error
}

type BiblioReads struct {
	client BiblioReadsClient
	index  GoodReadsIndex
}

func NewBiblioReads(client BiblioReadsClient, idx GoodReadsIndex) *BiblioReads {
	return &BiblioReads{client: client, index: idx}
}

func (b *BiblioReads) Name() string {
	return "Goodreads"
}

// AuthorLink returns the indexed author link without making a network request.
func (b *BiblioReads) AuthorLink(author index.Author, _ string) datasourcemodel.DataSourceLink {
	return goodReadsDataSourceLink(author.DataSourceLinks["goodreads"], "/author/show/")
}

// DocumentLink returns the indexed book link without making a network request.
func (b *BiblioReads) DocumentLink(document index.Document) datasourcemodel.DataSourceLink {
	return goodReadsDataSourceLink(document.DataSourceLink, "/book/show/")
}

func goodReadsDataSourceLink(path, prefix string) datasourcemodel.DataSourceLink {
	link := datasourcemodel.DataSourceLink{Source: "Goodreads", Icon: "/images/goodreads.svg", IconInvertible: true}
	if id, ok := strings.CutPrefix(path, prefix); ok && id != "" {
		link.URL = goodReadsBaseURL + path
	}
	return link
}

// CanEditDocumentLink reports that Goodreads book links can be corrected.
func (b *BiblioReads) CanEditDocumentLink() bool {
	return true
}

// UpdateDocumentLink saves a corrected Goodreads book link and clears stale score details.
func (b *BiblioReads) UpdateDocumentLink(document index.Document, link string) error {
	path, err := normalizeGoodReadsBookURL(link)
	if err != nil {
		return fmt.Errorf("%w: invalid Goodreads book URL", ErrInvalidDocumentLink)
	}
	if err := b.index.UpdateDataSourceLink(document.ID, path); err != nil {
		if errors.Is(err, index.ErrDocumentNotFound) {
			return ErrDocumentNotFound
		}
		return err
	}
	return nil
}

func normalizeGoodReadsBookURL(rawURL string) (string, error) {
	bookURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || len(rawURL) > 2048 || bookURL.User != nil ||
		(bookURL.Scheme != "https" && bookURL.Scheme != "http") ||
		(bookURL.Host != "www.goodreads.com" && bookURL.Host != "goodreads.com") ||
		!goodReadsBookPath.MatchString(bookURL.Path) {
		return "", fmt.Errorf("invalid Goodreads book URL")
	}
	return strings.TrimSuffix(bookURL.Path, "/"), nil
}

func bookID(bookURL string) (string, error) {
	id, ok := strings.CutPrefix(bookURL, goodReadsBookPrefix)
	if !ok || id == "" {
		return "", fmt.Errorf("invalid Goodreads book URL %q", bookURL)
	}
	return id, nil
}

func (b *BiblioReads) book(ctx context.Context, document index.Document) (biblioreads.BookResponse, error) {
	id, err := bookID(document.DataSourceLink)
	if err != nil {
		return biblioreads.BookResponse{}, err
	}
	book, err := b.client.Book(ctx, id)
	if err != nil {
		return book, fmt.Errorf("retrieving Goodreads book %s: %w", id, err)
	}
	details := book
	if book.Rating != "" && document.DataSourceScoreValue > 0 && isZeroRating(book.Rating) {
		book.Rating = fmt.Sprintf("%.2f", document.DataSourceScoreValue)
		details.Rating = ""
	}
	if err := b.index.StoreGoodReadsDetails(document, document.DataSourceLink, details); err != nil {
		log.Printf("Error saving Goodreads details for %s: %s", document.ID, err)
	}
	return book, nil
}

func isZeroRating(rating string) bool {
	value, err := strconv.ParseFloat(rating, 64)
	return err == nil && value == 0
}

func (b *BiblioReads) Score(ctx context.Context, document index.Document) (Score, error) {
	score := Score{Source: "Goodreads"}
	if document.DataSourceLink == "" {
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
		document.DataSourceLink = result.BookURL
		score.Value, score.Count, score.URL = result.Rating, result.RatingCount, b.DocumentLink(document).URL
		return score, nil
	}
	book, err := b.book(ctx, document)
	if err != nil {
		return score, err
	}
	score.Value, score.Count, score.URL = book.Rating, book.RatingCount, b.DocumentLink(document).URL
	return score, nil
}

func (b *BiblioReads) Reviews(ctx context.Context, document index.Document, language string) (Page, error) {
	if document.DataSourceLink == "" {
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
	page := Page{Source: "Goodreads", URL: b.DocumentLink(document).URL, Available: true}
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
