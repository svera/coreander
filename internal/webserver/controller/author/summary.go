package author

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/datasource/model"
	"github.com/svera/coreander/v4/internal/index"
)

func (a *Controller) Summary(c *fiber.Ctx) error {
	var (
		authorDataSource model.Author
		err              error
	)

	authorSlug := c.Params("slug")
	lang := c.Locals("Lang").(string)
	supportedLanguages := c.Locals("SupportedLanguages").([]string)
	template := "partials/author-summary"
	if c.Query("style") == "clear" {
		template = "partials/author-summary-doc-detail"
	}

	if authorSlug == "" {
		return fiber.ErrBadRequest
	}

	author, err := a.idx.Author(authorSlug, lang)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if author.Name == "" {
		return fiber.ErrNotFound
	}

	if !author.RetrievedOn.IsZero() {
		templateVars := fiber.Map{
			"Author": author,
		}

		if err = c.Render(template, templateVars); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		return nil
	}

	authorDataSource, err = a.dataSource.SearchAuthor(author.Name, supportedLanguages)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if authorDataSource == nil {
		return fiber.ErrNotFound
	}

	combineWithDataSource(&author, authorDataSource, supportedLanguages)

	if err := a.idx.IndexAuthor(author); err != nil {
		log.Println(err)
	}

	templateVars := fiber.Map{
		"Author": author,
	}

	if err = c.Render(template, templateVars); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}

func combineWithDataSource(author *index.Author, authorDataSource model.Author, supportedLanguages []string) {
	author.DataSourceID = authorDataSource.SourceID()
	author.BirthName = authorDataSource.BirthName()
	author.RetrievedOn = authorDataSource.RetrievedOn()
	author.WikipediaLink = make(map[string]string)
	author.InstanceOf = authorDataSource.InstanceOf()
	author.Description = make(map[string]string)
	author.DateOfBirth = authorDataSource.DateOfBirth()
	author.DateOfDeath = authorDataSource.DateOfDeath()
	author.Website = authorDataSource.Website()
	author.DataSourceImage = authorDataSource.Image()
	author.Gender = authorDataSource.Gender()
	author.Pseudonyms = make([]string, 0, len(authorDataSource.Pseudonyms()))

	for _, pseudonym := range authorDataSource.Pseudonyms() {
		if pseudonym != author.Name {
			author.Pseudonyms = append(author.Pseudonyms, pseudonym)
		}
	}

	for _, lang := range supportedLanguages {
		author.WikipediaLink[lang] = authorDataSource.WikipediaLink(lang)
		author.Description[lang] = authorDataSource.Description(lang)
	}
}
