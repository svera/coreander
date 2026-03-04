package document

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// Path prefixes for referers that get a "Return" back link on document detail.
// For "/documents" we only show the link when the referer has a query string (e.g. filters applied).
var backLinkPathPrefixes = []string{"/authors", "/documents", "/highlights", "/series"}

// backLinkFromReferer returns a URL path (and optional query) for the "Return" link when the
// referer is one of the allowed routes; otherwise it returns an empty string.
func backLinkFromReferer(referer string) string {
	if referer == "" {
		return ""
	}
	parsed, err := url.Parse(referer)
	if err != nil {
		return ""
	}
	path := parsed.Path
	query := parsed.RawQuery
	showBack := false
	for _, prefix := range backLinkPathPrefixes {
		if strings.HasPrefix(path, prefix) {
			showBack = true
			break
		}
	}
	if showBack && path == "/documents" && query == "" {
		showBack = false
	}
	// Do not show back link when coming from the document reader
	if showBack && strings.HasPrefix(path, "/documents/") && strings.HasSuffix(path, "/read") {
		showBack = false
	}
	if !showBack {
		return ""
	}
	if query != "" {
		return path + "?" + query
	}
	return path
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

	if _, err := os.Stat(filepath.Join(d.config.LibraryPath, document.ID)); err != nil {
		return fiber.ErrNotFound
	}

	title := document.Title
	if len(document.Authors) > 0 {
		title = fmt.Sprintf("%s - %s", strings.Join(document.Authors, ", "), document.Title)
	}

	backLink := backLinkFromReferer(string(c.RequestCtx().Referer()))

	sameSubjects, sameAuthors, sameSeries := d.related(document.Slug, int(session.ID))

	var completedOn *time.Time
	result := model.AugmentedDocument{Document: document}
	if session.ID > 0 {
		result = d.hlRepository.Highlighted(int(session.ID), result)
		completedOn, err = d.readingRepository.CompletedOn(int(session.ID), result.ID)
		if err != nil {
			log.Println(err)
		}
	}

	result.CompletedOn = completedOn
	return c.Render("document/detail", fiber.Map{
		"Title":          title,
		"BackLink":       backLink,
		"Document":       result,
		"EmailFrom":      d.sender.From(),
		"SameSeries":     sameSeries,
		"SameAuthors":    sameAuthors,
		"SameSubjects":   sameSubjects,
		"WordsPerMinute": d.config.WordsPerMinute,
	}, "layout")
}

func (d *Controller) related(slug string, sessionID int) (sameSubjects, sameAuthors, sameSeries []model.AugmentedDocument) {
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

	var authors []index.Document
	if authors, err = d.idx.SameAuthors(slug, relatedDocuments); err != nil {
		fmt.Println(err)
	}
	for i := range authors {
		result := model.AugmentedDocument{Document: authors[i]}
		result = d.hlRepository.Highlighted(sessionID, result)
		sameAuthors = append(sameAuthors, result)
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
	return sameSubjects, sameAuthors, sameSeries
}
