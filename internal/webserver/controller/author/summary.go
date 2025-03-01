package author

import (
	"log"

	"github.com/gofiber/fiber/v2"
)

func (a *Controller) Summary(c *fiber.Ctx) error {
	var (
		authorDataSource Author
		err              error
	)

	authorSlug := c.Params("slug")
	lang := c.Locals("Lang").(string)

	if authorSlug == "" {
		return fiber.ErrBadRequest
	}

	author, _ := a.idx.Author(authorSlug, lang)

	if author.DataSourceID != "" {
		templateVars := fiber.Map{
			"Author": author,
		}

		if err = c.Render("partials/author-summary", templateVars); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		return nil
	}

	if !author.RetrievedOn.IsZero() {
		return fiber.ErrNotFound
	}
	authorDataSource, err = a.dataSource.SearchAuthor(author.Name, c.Locals("SupportedLanguages").([]string))

	if err != nil {
		log.Println(err)
	}

	if authorDataSource == nil {
		return fiber.ErrNotFound
	}

	author.DataSourceID = authorDataSource.SourceID()
	author.Name = authorDataSource.Name(lang)
	author.BirthName = authorDataSource.BirthName()
	author.RetrievedOn = authorDataSource.RetrievedOn()
	author.WikipediaLink[lang] = authorDataSource.WikipediaLink(lang)
	author.InstanceOf = authorDataSource.InstanceOf()
	author.Description[lang] = authorDataSource.Description(lang)
	author.DateOfBirth = authorDataSource.DateOfBirth()
	author.YearOfBirth = authorDataSource.YearOfBirth()
	author.DateOfDeath = authorDataSource.DateOfDeath()
	author.YearOfDeath = authorDataSource.YearOfDeath()
	author.Website = authorDataSource.Website()
	author.Image = authorDataSource.Image()
	author.Gender = authorDataSource.Gender()
	author.Pseudonyms = authorDataSource.Pseudonyms()

	if err := a.idx.IndexAuthor(author); err != nil {
		log.Println(err)
	}

	templateVars := fiber.Map{
		"Author": author,
	}

	if err = c.Render("partials/author-summary", templateVars); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}
