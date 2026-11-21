package author

import (
	"context"
	"html/template"
	"log"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/microcosm-cc/bluemonday"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/webserver/model"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// BioProvider retrieves author biographies in the reader's language.
type BioProvider interface {
	ProfileInputProvider
	ProfileInputName() string
	NormalizeAuthorID(value string) (string, error)
	// StoredBio returns an indexed biography without network requests, and whether Bio must be called to get it.
	// Both methods also return the original language of the biography if it is an automatic translation.
	StoredBio(author index.Author, language string) (bio, translatedFrom string, pending bool)
	Bio(ctx context.Context, author index.Author, language string) (bio, translatedFrom string, err error)
	// RefreshBio retrieves the biography again, replacing the stored one and its translations.
	RefreshBio(ctx context.Context, author index.Author, language string) (bio, translatedFrom string, err error)
	UpdateBio(ctx context.Context, author index.Author, id, language string) (bio, translatedFrom string, err error)
}

// SetBioProvider enables author biographies on author pages.
func (a *Controller) SetBioProvider(provider BioProvider) {
	a.bioProvider = provider
}

// Bio returns the biography of an author, translated to the reader's language.
func (a *Controller) Bio(c fiber.Ctx) error {
	return a.renderBio(c, BioProvider.Bio)
}

// RefreshBio retrieves the biography of an author again, discarding the stored one.
func (a *Controller) RefreshBio(c fiber.Ctx) error {
	if a.bioProvider == nil {
		return c.SendString("")
	}
	inputName := a.bioProvider.ProfileInputName()
	if c.Request().PostArgs().Has(inputName) {
		id, err := a.bioProvider.NormalizeAuthorID(c.FormValue(inputName))
		if err != nil {
			log.Printf("Invalid author profile for %s: %s", c.Params("slug"), err)
			return a.renderInvalidProfileUpdate(c, inputName)
		}
		return a.renderBio(c, func(provider BioProvider, ctx context.Context, author index.Author, lang string) (string, string, error) {
			return provider.UpdateBio(ctx, author, id, lang)
		})
	}
	return a.renderBio(c, BioProvider.RefreshBio)
}

func (a *Controller) renderInvalidProfileUpdate(c fiber.Ctx, inputName string) error {
	lang := c.Locals("Lang").(string)
	author, err := a.idx.Author(c.Params("slug"), lang)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	if author.Name == "" {
		return fiber.ErrNotFound
	}
	c.Status(fiber.StatusBadRequest)
	return a.renderAuthorSummary(c, author, lang, inputName)
}

func (a *Controller) renderAuthorSummary(c fiber.Ctx, author index.Author, lang, invalidProfile string) error {
	session, _ := c.Locals("Session").(model.Session)
	vars := fiber.Map{
		"Author": author, "Lang": lang, "Session": session,
		"DataSourceLinks": a.authorDataSourceLinks(author, lang),
		"ImageVersion":    a.getImageVersion(author.Slug),
		"ProfileInputs":   a.profileInputs(author, "", invalidProfile, c.FormValue(invalidProfile)),
		"InvalidProfile":  invalidProfile != "",
	}
	a.addBioVars(vars, "partials/author-summary", author, lang)
	if err := c.Render("partials/author-summary", vars); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}

type bioFunc func(BioProvider, context.Context, index.Author, string) (string, string, error)

func (a *Controller) renderBio(c fiber.Ctx, retrieve bioFunc) error {
	if a.bioProvider == nil {
		return c.SendString("")
	}
	lang := c.Locals("Lang").(string)
	author, err := a.idx.Author(c.Params("slug"), lang)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	if author.Name == "" {
		return fiber.ErrNotFound
	}
	var bio, sourceLanguage string
	if c.Method() == fiber.MethodGet && c.Query("original") == "true" {
		bio, _, _ = a.bioProvider.StoredBio(author, author.Bio.Language)
	} else {
		bio, sourceLanguage, err = retrieve(a.bioProvider, c.Context(), author, lang)
	}
	if err != nil {
		log.Printf("Error retrieving biography for author %s: %s", author.Slug, err)
		if c.Request().PostArgs().Has(a.bioProvider.ProfileInputName()) {
			return fiber.ErrBadGateway
		}
	}
	session, _ := c.Locals("Session").(model.Session)
	if c.Request().PostArgs().Has(a.bioProvider.ProfileInputName()) {
		author, err = a.idx.Author(author.Slug, lang)
		if err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		return a.renderAuthorSummary(c, author, lang, "")
	}
	if bio == "" && (!a.bioRefreshAvailable(author) || session.Role != model.RoleAdmin) {
		return c.SendString("")
	}
	templateVars := fiber.Map{
		"Lang":           lang,
		"Slug":           author.Slug,
		"Session":        session,
		"Paragraphs":     bioParagraphs(bio),
		"TranslatedFrom": languageName(sourceLanguage, lang),
	}
	if err := c.Render("partials/author-bio", templateVars); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}

// storedBio returns the template variables for the author page biography.
func (a *Controller) storedBio(author index.Author, lang string) (paragraphs [][]template.HTML, translatedFromLanguage string, pending bool) {
	if a.bioProvider == nil {
		return nil, "", false
	}
	bio, sourceLanguage, pending := a.bioProvider.StoredBio(author, lang)
	return bioParagraphs(bio), languageName(sourceLanguage, lang), pending
}

// languageName returns the name of code in the reader's language, falling back to code itself.
func languageName(code, lang string) string {
	if code == "" {
		return ""
	}
	tag, err := language.Parse(code)
	if err != nil {
		return code
	}
	if name := display.Tags(language.Make(lang)).Name(tag); name != "" {
		return name
	}
	return code
}

// bioParagraphs splits a biography into paragraphs, each one made of lines.
var bioFormatting = bluemonday.NewPolicy().AllowElements("b", "strong", "i", "em")

func bioParagraphs(bio string) [][]template.HTML {
	var paragraphs [][]template.HTML
	for paragraph := range strings.SplitSeq(bio, "\n\n") {
		if paragraph = strings.TrimSpace(paragraph); paragraph != "" {
			var lines []template.HTML
			for _, line := range strings.Split(paragraph, "\n") {
				lines = append(lines, template.HTML(bioFormatting.Sanitize(line)))
			}
			paragraphs = append(paragraphs, lines)
		}
	}
	return paragraphs
}

// addBioVars adds the biography template variables used by the author card on the author page.
func (a *Controller) addBioVars(templateVars fiber.Map, template string, author index.Author, lang string) {
	if template != "partials/author-summary" {
		return
	}
	templateVars["BioParagraphs"], templateVars["BioTranslatedFrom"], templateVars["BioPending"] = a.storedBio(author, lang)
	templateVars["BioRefreshAvailable"] = a.bioRefreshAvailable(author)
}

func (a *Controller) bioRefreshAvailable(author index.Author) bool {
	return a.bioProvider != nil
}
