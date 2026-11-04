package biblioreads

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	datasourcemodel "github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/index"
)

const goodreadsBookPrefix = "/book/show/"

type ReviewClient interface {
	Book(ctx context.Context, id string) (BookResponse, error)
	Reviews(ctx context.Context, resourceID string, legacyBookID int, nextPageToken, languageCode string) (ReviewsResponse, error)
}

type ReviewIndex interface {
	Book(ctx context.Context, document index.Document) (SearchResult, error)
	StoreDetails(document index.Document, bookURL string, book BookResponse) error
	UpdateDataSourceLink(documentID, bookURL string) error
}

type ReviewProvider struct {
	client ReviewClient
	index  ReviewIndex
}

// NewReviewProvider exposes BiblioReads data using the shared review models.
func NewReviewProvider(client ReviewClient, idx ReviewIndex) *ReviewProvider {
	return &ReviewProvider{client: client, index: idx}
}

func (b *ReviewProvider) Name() string {
	return "Goodreads"
}

// AuthorLink returns the indexed author link without making a network request.
func (b *ReviewProvider) AuthorLink(author index.Author, _ string) datasourcemodel.DataSourceLink {
	return goodreadsDataSourceLink(author.DataSourceLinks["goodreads"], "/author/show/")
}

// DocumentLink returns the indexed book link without making a network request.
func (b *ReviewProvider) DocumentLink(document index.Document) datasourcemodel.DataSourceLink {
	data, err := index.DataSourceData[DocumentData](document, Source)
	if err != nil {
		log.Println(err)
		return datasourcemodel.DataSourceLink{}
	}
	return goodreadsDataSourceLink(data.URL, "/book/show/")
}

func goodreadsDataSourceLink(path, prefix string) datasourcemodel.DataSourceLink {
	link := datasourcemodel.DataSourceLink{Source: "Goodreads", Icon: "/images/goodreads.svg", IconInvertible: true}
	if id, ok := strings.CutPrefix(path, prefix); ok && id != "" {
		link.URL = goodreadsBaseURL + path
	}
	return link
}

// StoredScore builds a score from indexed data without making a network request.
func (b *ReviewProvider) StoredScore(document index.Document) (datasourcemodel.Score, error) {
	data, err := index.DataSourceData[DocumentData](document, Source)
	if err != nil {
		return datasourcemodel.Score{}, err
	}
	return datasourcemodel.Score{
		Source: b.Name(), Value: fmt.Sprintf("%.2f", document.DataSourceScoreValue),
		Count: data.RatingCount, URL: goodreadsDataSourceLink(data.URL, goodreadsBookPrefix).URL,
	}, nil
}

// CanEditDocumentLink reports that Goodreads book links can be corrected.
func (b *ReviewProvider) CanEditDocumentLink() bool {
	return true
}

// UpdateDocumentLink saves a corrected Goodreads book link and clears stale score details.
func (b *ReviewProvider) UpdateDocumentLink(document index.Document, link string) error {
	path, err := normalizeGoodreadsBookURL(link)
	if err != nil {
		return fmt.Errorf("%w: invalid Goodreads book URL", datasourcemodel.ErrInvalidDocumentLink)
	}
	if err := b.index.UpdateDataSourceLink(document.ID, path); err != nil {
		if errors.Is(err, index.ErrDocumentNotFound) {
			return datasourcemodel.ErrDocumentNotFound
		}
		return err
	}
	return nil
}

func normalizeGoodreadsBookURL(rawURL string) (string, error) {
	bookURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || len(rawURL) > 2048 || bookURL.User != nil ||
		(bookURL.Scheme != "https" && bookURL.Scheme != "http") ||
		(bookURL.Host != "www.goodreads.com" && bookURL.Host != "goodreads.com") ||
		!goodreadsBookPath.MatchString(bookURL.Path) {
		return "", fmt.Errorf("invalid Goodreads book URL")
	}
	return strings.TrimSuffix(bookURL.Path, "/"), nil
}

func bookID(bookURL string) (string, error) {
	id, ok := strings.CutPrefix(bookURL, goodreadsBookPrefix)
	if !ok || id == "" {
		return "", fmt.Errorf("invalid Goodreads book URL %q", bookURL)
	}
	return id, nil
}

func (b *ReviewProvider) book(ctx context.Context, document index.Document) (BookResponse, error) {
	data, err := index.DataSourceData[DocumentData](document, Source)
	if err != nil {
		return BookResponse{}, err
	}
	id, err := bookID(data.URL)
	if err != nil {
		return BookResponse{}, err
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
	if err := b.index.StoreDetails(document, data.URL, details); err != nil {
		log.Printf("Error saving Goodreads details for %s: %s", document.ID, err)
	}
	return book, nil
}

func isZeroRating(rating string) bool {
	value, err := strconv.ParseFloat(rating, 64)
	return err == nil && value == 0
}

func (b *ReviewProvider) Score(ctx context.Context, document index.Document) (datasourcemodel.Score, error) {
	score := datasourcemodel.Score{Source: "Goodreads"}
	data, err := index.DataSourceData[DocumentData](document, Source)
	if err != nil {
		return score, err
	}
	if data.URL == "" {
		result, err := b.index.Book(ctx, document)
		if err != nil {
			return score, err
		}
		if result.BookURL == "" {
			return score, nil
		}
		if _, err := bookID(result.BookURL); err != nil {
			return score, err
		}
		data.URL = result.BookURL
		if err := index.SetDataSourceData(&document, Source, data); err != nil {
			return score, err
		}
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

func (b *ReviewProvider) Reviews(ctx context.Context, document index.Document, language string) (datasourcemodel.ReviewPage, error) {
	data, err := index.DataSourceData[DocumentData](document, Source)
	if err != nil {
		return datasourcemodel.ReviewPage{}, err
	}
	if data.URL == "" {
		return datasourcemodel.ReviewPage{}, nil
	}
	book, err := b.book(ctx, document)
	if err != nil {
		return datasourcemodel.ReviewPage{}, err
	}
	if book.Rating == "" {
		return datasourcemodel.ReviewPage{}, nil
	}
	resourceID, legacyBookID := data.ResourceID, data.LegacyBookID
	if book.ResourceID != "" && book.LegacyBookID > 0 {
		resourceID, legacyBookID = book.ResourceID, book.LegacyBookID
	}
	if resourceID == "" || legacyBookID <= 0 {
		return datasourcemodel.ReviewPage{}, fmt.Errorf("missing Goodreads review identifiers for %s", document.ID)
	}
	response, err := b.client.Reviews(ctx, resourceID, legacyBookID, "", language)
	if err != nil {
		return datasourcemodel.ReviewPage{}, fmt.Errorf("retrieving Goodreads reviews for %s: %w", document.ID, err)
	}
	page := datasourcemodel.ReviewPage{Source: "Goodreads", URL: b.DocumentLink(document).URL, Available: true}
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
			reviewURL = goodreadsBaseURL + review.ReviewURL
		}
		page.Reviews = append(page.Reviews, datasourcemodel.Review{
			Reviewer: review.Reviewer.Name, Rating: review.Rating, Text: review.Text,
			CreatedAt: createdAt, Spoiler: review.Spoiler, URL: reviewURL,
		})
	}
	return page, nil
}
