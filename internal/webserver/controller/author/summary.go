package author

import (
	"log"

	"github.com/gofiber/fiber/v2"
)

func (a *Controller) Summary(c *fiber.Ctx) error {
	var (
		authorData Author
		err        error
	)

	authorSlug := c.Params("slug")

	if authorSlug == "" {
		return fiber.ErrBadRequest
	}

	author, _ := a.idx.Author(authorSlug)

	if author.WikidataID != "" {
		authorData, err = a.dataSource.RetrieveAuthor(author.WikidataID, c.Locals("Lang").(string))
	} else {
		if !author.RetrievedOn.IsZero() {
			return fiber.ErrNotFound
		}
		authorData, err = a.dataSource.SearchAuthor(author.Name, c.Locals("Lang").(string))
	}
	if err != nil {
		log.Println(err)
	}

	if authorData == nil {
		return fiber.ErrNotFound
	}

	author.WikidataID = authorData.SourceID()
	author.RetrievedOn = authorData.RetrievedOn()
	if err := a.idx.IndexAuthor(author); err != nil {
		log.Println(err)
	}

	templateVars := fiber.Map{
		"Author":     author,
		"AuthorData": authorData,
	}

	if err = c.Render("partials/author-summary", templateVars); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}
