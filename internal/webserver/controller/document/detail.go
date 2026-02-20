package document

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

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
