package author

import (
	"log"

	"github.com/gofiber/fiber/v3"
	datasourcemodel "github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/webserver/controller/fsutil"
)

func (a *Controller) Summary(c fiber.Ctx) error {
	// Set cache headers to prevent caching of author summary HTML
	// This ensures fresh ImageVersion is always retrieved
	c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")
	var (
		authorDataSource datasourcemodel.Author
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

	// Get image cache version for cache busting
	imageVersion := a.getImageVersion(author.Slug)

	if !author.RetrievedOn.IsZero() {
		templateVars := fiber.Map{
			"Author":       author,
			"ImageVersion": imageVersion,
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

	index.CombineWithDataSource(&author, authorDataSource, supportedLanguages)

	if err := a.idx.IndexAuthor(author); err != nil {
		log.Println(err)
	}

	templateVars := fiber.Map{
		"Author":       author,
		"ImageVersion": imageVersion,
	}

	if err = c.Render(template, templateVars); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}

// getImageVersion returns the modification time of the cached image file as a cache-busting version
// Returns empty string if file doesn't exist
func (a *Controller) getImageVersion(authorSlug string) string {
	return fsutil.AuthorImageVersion(a.appFs, a.config.CacheDir, authorSlug)
}
