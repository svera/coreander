package home

import (
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
)

type Sender interface {
	SendDocument(address, subject, libraryPath, fileName string) error
	From() string
}

// IdxReaderWriter defines a set of reading and writing operations over an index
type IdxReaderWriter interface {
	DocumentByID(ID string) (index.Document, error)
	Count(t string) (uint64, error)
	LatestDocs(limit int) ([]index.Document, error)
	Languages() ([]string, error)
}

type highlightsRepository interface {
	Highlighted(userID int, doc index.Document) index.Document
}

type readingRepository interface {
	Latest(userID int, page int, resultsPerPage int) (result.Paginated[[]string], error)
	Completed(userID int, doc index.Document) index.Document
}

type Config struct {
	LibraryPath     string
	CoverMaxWidth   int
	LatestDocsLimit int
}

type Controller struct {
	hlRepository      highlightsRepository
	readingRepository readingRepository
	idx               IdxReaderWriter
	sender            Sender
	config            Config
}

func NewController(hlRepository highlightsRepository, readingRepository readingRepository, sender Sender, idx IdxReaderWriter, cfg Config) *Controller {
	return &Controller{
		hlRepository:      hlRepository,
		readingRepository: readingRepository,
		idx:               idx,
		sender:            sender,
		config:            cfg,
	}
}
