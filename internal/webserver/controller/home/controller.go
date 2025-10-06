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
}

type highlightsRepository interface {
	Highlighted(userID int, doc index.Document) index.Document
}

type historyRepository interface {
	LatestReads(userID int, page int, resultsPerPage int) (result.Paginated[[]string], error)
}

type Config struct {
	LibraryPath     string
	CoverMaxWidth   int
	LatestDocsLimit int
}

type Controller struct {
	hlRepository      highlightsRepository
	historyRepository historyRepository
	idx               IdxReaderWriter
	sender            Sender
	config            Config
}

func NewController(hlRepository highlightsRepository, historyRepository historyRepository, sender Sender, idx IdxReaderWriter, cfg Config) *Controller {
	return &Controller{
		hlRepository:      hlRepository,
		historyRepository: historyRepository,
		idx:               idx,
		sender:            sender,
		config:            cfg,
	}
}
