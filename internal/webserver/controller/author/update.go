package author

import (
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/index"
)

func (a *Controller) Update(c *fiber.Ctx) error {
	authorSlug := c.Params("slug")
	supportedLanguages := c.Locals("SupportedLanguages").([]string)
	lang := c.Locals("Lang").(string)
	sourceID := c.FormValue("sourceID")

	if authorSlug == "" {
		return fiber.ErrBadRequest
	}

	author, err := a.idx.Author(authorSlug, lang)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if author.Slug == "" {
		return fiber.ErrNotFound
	}

	if sourceID == "" {
		author = clear(author)
	} else {
		authorDataSource, err := a.dataSource.RetrieveAuthor([]string{sourceID}, supportedLanguages)
		if err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}

		if authorDataSource == nil {
			return fiber.ErrNotFound
		}

		if err := a.appFs.Remove(a.config.CacheDir + "/" + author.Slug + ".jpg"); err != nil {
			fmt.Println(err)
		}

		combineWithDataSource(&author, authorDataSource, supportedLanguages)
	}

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

func clear(author index.Author) index.Author {
	cleanAuthor := index.Author{}
	cleanAuthor.Slug = author.Slug
	cleanAuthor.Name = author.Name
	cleanAuthor.RetrievedOn = time.Now().UTC()
	return cleanAuthor
}
