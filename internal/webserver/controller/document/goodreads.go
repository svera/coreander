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
// responses don't block the page. Renders nothing if the rating can't be
// retrieved.
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
	// Searches the document first if it hasn't been linked to Goodreads yet.
	bookURL, err := d.idx.GoodReadsBookURL(ctx, document)
	if err != nil {
		log.Println(err)
		return c.SendString("")
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
	if book.Rating == "" {
		return c.SendString("")
	}

	return c.Render("partials/document-goodreads", fiber.Map{
		"Rating":      book.Rating,
		"RatingCount": book.RatingCount,
		"URL":         goodReadsBaseURL + bookURL,
	})
}
