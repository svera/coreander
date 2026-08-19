package document

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gosimple/slug"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/webserver/controller/fsutil"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

// authorSummary bundles an author with the data its summary partial needs to
// render eagerly (i.e. without the async /summary htmx round-trip), so it can
// only fall back to that round-trip when the author hasn't been enriched yet.
type authorSummary struct {
	Author       index.Author
	ImageVersion string
}

func (d *Controller) Detail(c fiber.Ctx) error {
	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.WordsPerMinute > 0 {
		d.config.WordsPerMinute = session.WordsPerMinute
	}

	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if document.Slug == "" {
		return fiber.ErrNotFound
	}

	title := document.Title
	if len(document.Authors) > 0 {
		title = fmt.Sprintf("%s - %s", strings.Join(document.Authors, ", "), document.Title)
	}

	lang, _ := c.Locals("Lang").(string)
	authorSummaries, illustratorSummaries := d.authorAndIllustratorSummaries(document, lang)

	sameSubjects, sameSeries := d.related(document.Slug, int(session.ID))

	var completedOn *time.Time
	result := model.AugmentedDocument{Document: document}
	if session.ID > 0 {
		result = d.hlRepository.Highlighted(int(session.ID), result)
		completedOn, err = d.readingRepository.CompletedOn(int(session.ID), result.Slug)
		if err != nil {
			log.Println(err)
		}
	}

	result.CompletedOn = completedOn
	return c.Render("document/detail", fiber.Map{
		"Title":                title,
		"Document":             result,
		"EmailFrom":            d.sender.From(),
		"SameSeries":           sameSeries,
		"SameSubjects":         sameSubjects,
		"WordsPerMinute":       d.config.WordsPerMinute,
		"AuthorSummaries":      authorSummaries,
		"IllustratorSummaries": illustratorSummaries,
	}, "layout")
}

// authorAndIllustratorSummaries resolves the (up to 2) authors and (up to 2)
// illustrators shown on a document's detail page, so their summary partials can
// render eagerly when the author is already enriched, skipping the async
// /summary htmx round-trip. Illustrators already listed as authors are skipped.
func (d *Controller) authorAndIllustratorSummaries(document index.Document, lang string) (authors, illustrators []authorSummary) {
	if len(document.Authors) > 0 && len(document.Authors) <= 2 && document.Authors[0] != "" {
		for i := range document.Authors {
			if i >= len(document.AuthorsSlugs) {
				continue
			}
			authors = append(authors, d.authorSummaryFor(document.AuthorsSlugs[i], lang))
		}
	}

	if len(document.Illustrators) > 0 && len(document.Illustrators) <= 2 && document.Illustrators[0] != "" {
		for _, illustrator := range document.Illustrators {
			if illustrator == "" {
				continue
			}
			illustratorSlug := slug.Make(illustrator)
			isAlsoAuthor := false
			for _, authorName := range document.Authors {
				if slug.Make(authorName) == illustratorSlug {
					isAlsoAuthor = true
					break
				}
			}
			if isAlsoAuthor {
				continue
			}
			illustrators = append(illustrators, d.authorSummaryFor(illustratorSlug, lang))
		}
	}

	return authors, illustrators
}

func (d *Controller) authorSummaryFor(authorSlug, lang string) authorSummary {
	author, err := d.idx.Author(authorSlug, lang)
	if err != nil {
		log.Println(err)
	}

	return authorSummary{Author: author, ImageVersion: fsutil.AuthorImageVersion(d.appFs, d.config.CacheDir, author.Slug)}
}

func (d *Controller) related(slug string, sessionID int) (sameSubjects, sameSeries []model.AugmentedDocument) {
	var err error
	var subjects []index.Document
	if subjects, err = d.idx.SameSubjects(slug, relatedDocuments); err != nil {
		fmt.Println(err)
	}
	for i := range subjects {
		result := model.AugmentedDocument{Document: subjects[i]}
		result = d.hlRepository.Highlighted(sessionID, result)
		sameSubjects = append(sameSubjects, result)
	}

	var series []index.Document
	if series, err = d.idx.SameSeries(slug, relatedDocuments); err != nil {
		fmt.Println(err)
	}
	for i := range series {
		result := model.AugmentedDocument{Document: series[i]}
		result = d.hlRepository.Highlighted(sessionID, result)
		sameSeries = append(sameSeries, result)
	}

	return sameSubjects, sameSeries
}
