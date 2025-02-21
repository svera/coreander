package author

import (
	"log"

	"github.com/gofiber/fiber/v2"
)

func (a *Controller) Update(c *fiber.Ctx) error {
	authorSlug := c.Params("slug")

	if authorSlug == "" {
		return fiber.ErrBadRequest
	}

	author, _ := a.idx.Author(authorSlug)
	authorData, err := a.dataSource.Retrieve(c.FormValue("sourceID"), c.Locals("Lang").(string))
	if err != nil {
		log.Println(err)
	}

	author.WikidataID = authorData.SourceID()
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
