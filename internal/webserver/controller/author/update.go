package author

import (
	"log"

	"github.com/gofiber/fiber/v2"
)

func (a *Controller) Update(c *fiber.Ctx) error {
	authorSlug := c.Params("slug")
	lang := c.Locals("Lang").(string)
	supportedLanguages := c.Locals("SupportedLanguages").([]string)

	if authorSlug == "" {
		return fiber.ErrBadRequest
	}

	author, _ := a.idx.Author(authorSlug, lang)
	authorData, err := a.dataSource.RetrieveAuthor([]string{c.FormValue("sourceID")}, supportedLanguages)
	if err != nil {
		log.Println(err)
		return fiber.ErrNotFound
	}

	author.DataSourceID = authorData.SourceID()
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
