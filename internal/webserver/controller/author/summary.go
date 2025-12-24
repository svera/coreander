package author

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/datasource/model"
	"github.com/svera/coreander/v4/internal/index"
	webservermodel "github.com/svera/coreander/v4/internal/webserver/model"
)

func (a *Controller) Summary(c *fiber.Ctx) error {
	// Set cache headers to prevent caching of author summary HTML
	// This ensures fresh ImageVersion is always retrieved
	c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")
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

	// Get session and image version (used in both branches)
	var session webservermodel.Session
	if val, ok := c.Locals("Session").(webservermodel.Session); ok {
		session = val
	}

	// Get image cache version for cache busting
	imageVersion := a.getImageVersion(author.Slug)

	if !author.RetrievedOn.IsZero() {
		templateVars := fiber.Map{
			"Author":       author,
			"Session":      session,
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

	combineWithDataSource(&author, authorDataSource, supportedLanguages)

	if err := a.idx.IndexAuthor(author); err != nil {
		log.Println(err)
	}

	templateVars := fiber.Map{
		"Author":       author,
		"Session":      session,
		"ImageVersion": imageVersion,
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

// getImageVersion returns the modification time of the cached image file as a cache-busting version
// Returns empty string if file doesn't exist
func (a *Controller) getImageVersion(authorSlug string) string {
	imageFileName := a.config.CacheDir + "/" + authorSlug + ".jpg"
	fileInfo, err := a.appFs.Stat(imageFileName)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("?t=%d", fileInfo.ModTime().Unix())
}
