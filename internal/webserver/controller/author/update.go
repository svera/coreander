package author

import (
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v3"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

func (a *Controller) Update(c fiber.Ctx) error {
	authorSlug := c.Params("slug")
	supportedLanguages := c.Locals("SupportedLanguages").([]string)
	lang := c.Locals("Lang").(string)
	session, _ := c.Locals("Session").(model.Session)
	sourceID, err := a.dataSource.NormalizeAuthorID(c.FormValue("sourceID"))
	if err != nil {
		log.Printf("Invalid author profile for %s: %s", authorSlug, err)
		return fiber.ErrBadRequest
	}
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

	if author.Slug == "" {
		return fiber.ErrNotFound
	}

	if sourceID == "" {
		author = author.ClearMetadata(a.dataSource.OwnsLink)
	} else {
		authorDataSource, err := a.dataSource.RetrieveAuthor([]string{sourceID}, supportedLanguages, date.Zero)
		if err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}

		if authorDataSource == nil {
			return fiber.ErrNotFound
		}

		index.CombineWithDataSource(&author, authorDataSource, supportedLanguages)
	}

	if err := a.appFs.Remove(a.config.CacheDir + "/authors/" + author.Slug + ".webp"); err != nil && !os.IsNotExist(err) {
		log.Println(fmt.Errorf("error removing cached image for author %s: %w", author.Slug, err))
		return fiber.ErrInternalServerError
	}

	if err := a.idx.IndexAuthor(author); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if provider, unified := a.metadataBioProvider(); unified {
		author, err = a.idx.Author(authorSlug, lang)
		if err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		if err := provider.RefreshMetadataBio(c.Context(), author, lang); err != nil {
			log.Printf("Error refreshing biography after metadata update for author %s: %s", authorSlug, err)
			return fiber.ErrInternalServerError
		}
		author, err = a.idx.Author(authorSlug, lang)
		if err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
	}

	templateVars := fiber.Map{
		"Author":                    author,
		"AuthorDataSourceReference": AuthorDataSourceReference(author, a.dataSource),
		"DataSourceLinks":           a.authorDataSourceLinks(author, lang),
		"ImageVersion":              a.getImageVersion(author.Slug),
		"Lang":                      lang,
		"Session":                   session,
		"ProfileInputs":             a.profileInputs(author, lang, c.Query("style"), "", ""),
	}
	a.addBioVars(templateVars, template, author, lang)

	if err = c.Render(template, templateVars); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}
