package author

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

func (a *Controller) Update(c fiber.Ctx) error {
	authorSlug := c.Params("slug")
	supportedLanguages := c.Locals("SupportedLanguages").([]string)
	lang := c.Locals("Lang").(string)
	session, _ := c.Locals("Session").(model.Session)
	sourceID, err := wikidataAuthorID(c.FormValue("sourceID"))
	if err != nil {
		log.Printf("Invalid Wikidata author ID for %s: %s", authorSlug, err)
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
		authorDataSource, err := a.dataSource.RetrieveAuthor([]string{sourceID}, supportedLanguages)
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

	templateVars := fiber.Map{
		"Author":          author,
		"DataSourceLinks": a.authorDataSourceLinks(author, lang),
		"ImageVersion":    a.getImageVersion(author.Slug),
		"Lang":            lang,
		"Session":         session,
		"ProfileInputs":   a.profileInputs(author, c.Query("style"), "", ""),
	}
	a.addBioVars(templateVars, template, author, lang)

	if err = c.Render(template, templateVars); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}

var wikidataIDPattern = regexp.MustCompile(`^[Qq][0-9]+$`)

func wikidataAuthorID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.Contains(value, "://") {
		target, err := url.Parse(value)
		if err != nil || (target.Scheme != "https" && target.Scheme != "http") ||
			(target.Host != "www.wikidata.org" && target.Host != "wikidata.org") ||
			target.User != nil || target.RawQuery != "" || target.Fragment != "" ||
			target.RawPath != "" || target.ForceQuery || strings.Contains(value, "#") {
			return "", fmt.Errorf("invalid Wikidata URL %q", value)
		}
		path := strings.TrimSuffix(target.Path, "/")
		switch {
		case strings.HasPrefix(path, "/wiki/"):
			value = strings.TrimPrefix(path, "/wiki/")
		case strings.HasPrefix(path, "/entity/"):
			value = strings.TrimPrefix(path, "/entity/")
		default:
			return "", fmt.Errorf("invalid Wikidata URL %q", value)
		}
	}
	if !wikidataIDPattern.MatchString(value) {
		return "", fmt.Errorf("invalid Wikidata ID %q", value)
	}
	return "Q" + value[1:], nil
}
