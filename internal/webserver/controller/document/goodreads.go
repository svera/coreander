package document

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
)

const (
	goodReadsBaseURL     = "https://www.goodreads.com"
	goodReadsBookPrefix  = "/book/show/"
	goodReadsBookTimeout = 30 * time.Second
)

// BookSource retrieves book details from Goodreads; implemented by *biblioreads.Client.
type BookSource interface {
	Book(ctx context.Context, id string) (biblioreads.BookResponse, error)
}

// GoodReads renders the Goodreads rating shown in a document's detail page
// metadata. It's loaded asynchronously via htmx, so slow BiblioReads
// responses don't block the page. Shows "no score" for an unmatched document,
// but renders nothing if the rating request fails.
func (d *Controller) GoodReads(c fiber.Ctx) error {
	if d.bookSource == nil {
		return c.SendString("")
	}

	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		log.Println(err)
		return c.SendString("")
	}
	if document.Slug == "" {
		return c.SendString("")
	}

	ctx, cancel := context.WithTimeout(c.Context(), goodReadsBookTimeout)
	defer cancel()
	bookURL := document.GoodReadsBookURL
	if bookURL == "" {
		result, err := d.idx.GoodReadsBook(ctx, document)
		if err != nil {
			log.Println(err)
			return c.SendString("")
		}
		if result.BookURL == "" {
			return c.Render("partials/document-goodreads", fiber.Map{})
		}
		if result.Rating == "" {
			return c.SendString("")
		}
		if id, ok := strings.CutPrefix(result.BookURL, goodReadsBookPrefix); !ok || id == "" {
			return c.SendString("")
		}
		return c.Render("partials/document-goodreads", fiber.Map{
			"Rating": result.Rating,
			"URL":    goodReadsBaseURL + result.BookURL,
		})
	}
	id, ok := strings.CutPrefix(bookURL, goodReadsBookPrefix)
	if !ok || id == "" {
		return c.SendString("")
	}

	book, err := d.bookSource.Book(ctx, id)
	if err != nil {
		log.Printf("Error retrieving GoodReads book %s: %s\n", id, err)
		return c.SendString("")
	}
	if book.ResourceID != "" && book.LegacyBookID > 0 {
		if err := d.idx.StoreGoodReadsBookIDs(document, bookURL, book.ResourceID, book.LegacyBookID); err != nil {
			log.Printf("Error saving GoodReads identifiers for %s: %s\n", document.ID, err)
		}
	}
	if book.Rating == "" {
		return c.SendString("")
	}

	return c.Render("partials/document-goodreads", fiber.Map{
		"Rating":      book.Rating,
		"RatingCount": book.RatingCount,
		"URL":         goodReadsBaseURL + bookURL,
	})
}
