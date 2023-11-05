package controller

import (
	"fmt"
	"io"
	"log"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v3/internal/infrastructure"
	"github.com/svera/coreander/v3/internal/jwtclaimsreader"
	"github.com/svera/coreander/v3/internal/metadata"
	"github.com/svera/coreander/v3/internal/model"
	"github.com/svera/coreander/v3/internal/search"
)

const relatedDocuments = 4

type Sender interface {
	SendDocument(address string, libraryPath string, fileName string) error
	From() string
}

// IdxReaderWriter defines a set of reading and writing operations over an index
type IdxReaderWriter interface {
	Search(keywords string, page, resultsPerPage int) (*search.PaginatedResult, error)
	Count() (uint64, error)
	Close() error
	Document(Slug string) (search.Document, error)
	Documents(IDs []string) ([]search.Document, error)
	SameSubjects(slug string, quantity int) ([]search.Document, error)
	SameAuthors(slug string, quantity int) ([]search.Document, error)
	SameSeries(slug string, quantity int) ([]search.Document, error)
	RemoveFile(file string) error
}

type Documents struct {
	hlRepository    highlightsRepository
	usrRepository   usersRepository
	idx             IdxReaderWriter
	sender          Sender
	wordsPerMinute  float64
	libraryPath     string
	homeDir         string
	metadataReaders map[string]metadata.Reader
	coverMaxWidth   int
	appFs           afero.Fs
}

type DocumentsConfig struct {
	WordsPerMinute float64
	LibraryPath    string
	HomeDir        string
	CoverMaxWidth  int
}

func NewDocuments(hlRepository highlightsRepository, usrRepository usersRepository, sender Sender, idx IdxReaderWriter, metadataReaders map[string]metadata.Reader, appFs afero.Fs, cfg DocumentsConfig) *Documents {
	return &Documents{
		hlRepository:    hlRepository,
		usrRepository:   usrRepository,
		idx:             idx,
		sender:          sender,
		wordsPerMinute:  cfg.WordsPerMinute,
		libraryPath:     cfg.LibraryPath,
		homeDir:         cfg.HomeDir,
		metadataReaders: metadataReaders,
		coverMaxWidth:   cfg.CoverMaxWidth,
		appFs:           appFs,
	}
}

func (d *Documents) Search(c *fiber.Ctx) error {
	emailSendingConfigured := true
	if _, ok := d.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	session := jwtclaimsreader.SessionData(c)
	if session.WordsPerMinute > 0 {
		d.wordsPerMinute = session.WordsPerMinute
	}

	var searchResults *search.PaginatedResult

	if keywords := c.Query("search"); keywords != "" {
		if searchResults, err = d.idx.Search(keywords, page, model.ResultsPerPage); err != nil {
			return fiber.ErrInternalServerError
		}

		if session.ID > 0 {
			searchResults.Hits = d.hlRepository.Highlighted(int(session.ID), searchResults.Hits)
		}

		return c.Render("results", fiber.Map{
			"Keywords":               keywords,
			"Results":                searchResults.Hits,
			"Total":                  searchResults.TotalHits,
			"Paginator":              pagination(model.MaxPagesNavigator, searchResults.TotalPages, searchResults.Page, map[string]string{"search": keywords}),
			"Title":                  "Search results",
			"EmailSendingConfigured": emailSendingConfigured,
			"EmailFrom":              d.sender.From(),
			"Session":                session,
			"WordsPerMinute":         d.wordsPerMinute,
		}, "layout")
	}

	count, err := d.idx.Count()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.Render("index", fiber.Map{
		"Count":   count,
		"Title":   "Coreander",
		"Session": session,
	}, "layout")
}

func (d *Documents) Document(c *fiber.Ctx) error {
	emailSendingConfigured := true
	if _, ok := d.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	session := jwtclaimsreader.SessionData(c)
	if session.WordsPerMinute > 0 {
		d.wordsPerMinute = session.WordsPerMinute
	}

	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		fmt.Println(err)
		return fiber.ErrBadRequest
	}

	if _, err := os.Stat(filepath.Join(d.libraryPath, document.ID)); err != nil {
		return fiber.ErrNotFound
	}

	title := fmt.Sprintf("%s | Coreander", document.Title)
	authors := strings.Join(document.Authors, ", ")
	if authors != "" {
		title = fmt.Sprintf("%s - %s | Coreander", authors, document.Title)
	}

	sameSubjects, err := d.idx.SameSubjects(document.Slug, relatedDocuments)
	if err != nil {
		fmt.Println(err)
	}

	sameAuthors, err := d.idx.SameAuthors(document.Slug, relatedDocuments)
	if err != nil {
		fmt.Println(err)
	}

	sameSeries, err := d.idx.SameSeries(document.Slug, relatedDocuments)
	if err != nil {
		fmt.Println(err)
	}

	if session.ID > 0 {
		document = d.hlRepository.Highlighted(int(session.ID), []search.Document{document})[0]
	}

	return c.Render("document", fiber.Map{
		"Title":                  title,
		"Document":               document,
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              d.sender.From(),
		"Session":                session,
		"SameSeries":             sameSeries,
		"SameAuthors":            sameAuthors,
		"SameSubjects":           sameSubjects,
		"WordsPerMinute":         d.wordsPerMinute,
	}, "layout")
}

func (d *Documents) Download(c *fiber.Ctx) error {
	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	fullPath := filepath.Join(d.libraryPath, document.ID)

	if _, err := os.Stat(fullPath); err != nil {
		return fiber.ErrNotFound
	}

	file, err := os.Open(fullPath)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	ext := strings.ToLower(filepath.Ext(document.ID))

	if ext == ".epub" {
		c.Response().Header.Set(fiber.HeaderContentType, "application/epub+zip")
	} else {
		c.Response().Header.Set(fiber.HeaderContentType, "application/pdf")
	}

	c.Response().Header.Set(fiber.HeaderContentDisposition, fmt.Sprintf("inline; filename=\"%s\"", filepath.Base(document.ID)))
	c.Response().BodyWriter().Write(contents)
	return nil
}

func (d *Documents) Cover(c *fiber.Ctx) error {
	c.Append("Cache-Time", "86400")

	var (
		image []byte
	)

	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}
	ext := filepath.Ext(document.ID)

	if _, ok := d.metadataReaders[ext]; !ok {
		return fiber.ErrBadRequest
	}
	image, err = d.metadataReaders[ext].Cover(filepath.Join(d.libraryPath, document.ID), d.coverMaxWidth)
	if err != nil {
		log.Println(err)
		return fiber.ErrNotFound
	}

	c.Response().Header.Set(fiber.HeaderContentType, "image/jpeg")
	c.Response().BodyWriter().Write(image)
	return nil
}

func (d *Documents) Delete(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin {
		return fiber.ErrForbidden
	}

	if c.FormValue("slug") == "" {
		return fiber.ErrBadRequest
	}

	document, err := d.idx.Document(c.FormValue("slug"))
	if err != nil {
		fmt.Println(err)
		return fiber.ErrBadRequest
	}

	fullPath := filepath.Join(d.libraryPath, document.ID)
	if _, err := d.appFs.Stat(fullPath); err != nil {
		return fiber.ErrBadRequest
	}

	if err := d.idx.RemoveFile(fullPath); err != nil {
		return fiber.ErrInternalServerError
	}

	if err := d.appFs.Remove(fullPath); err != nil {
		log.Printf("error removing file %s", fullPath)
	}

	return nil
}

func (d *Documents) Send(c *fiber.Ctx) error {
	if strings.Trim(c.FormValue("slug"), " ") == "" {
		return fiber.ErrBadRequest
	}

	if _, err := mail.ParseAddress(c.FormValue("email")); err != nil {
		return fiber.ErrBadRequest
	}

	document, err := d.idx.Document(c.FormValue("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	if _, err := os.Stat(filepath.Join(d.libraryPath, document.ID)); err != nil {
		return fiber.ErrBadRequest
	}

	return d.sender.SendDocument(c.FormValue("email"), d.libraryPath, document.ID)
}

func (d *Documents) DocReader(c *fiber.Ctx) error {
	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		fmt.Println(err)
		return fiber.ErrBadRequest
	}

	if _, err := os.Stat(filepath.Join(d.libraryPath, document.ID)); err != nil {
		return fiber.ErrNotFound
	}

	template := "epub-reader"
	if strings.ToLower(filepath.Ext(document.ID)) == ".pdf" {
		template = "pdf-reader"
	}

	title := fmt.Sprintf("%s | Coreander", document.Title)
	authors := strings.Join(document.Authors, ", ")
	if authors != "" {
		title = fmt.Sprintf("%s - %s | Coreander", authors, document.Title)
	}
	return c.Render(template, fiber.Map{
		"Title":       title,
		"Author":      strings.Join(document.Authors, ", "),
		"Description": document.Description,
		"Slug":        document.Slug,
	})

}
