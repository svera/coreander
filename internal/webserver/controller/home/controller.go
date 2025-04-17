package home

import (
	"github.com/svera/coreander/v4/internal/index"
)

type Sender interface {
	SendDocument(address, subject, libraryPath, fileName string) error
	From() string
}

// IdxReaderWriter defines a set of reading and writing operations over an index
type IdxReaderWriter interface {
	Documents(IDs []string) (map[string]index.Document, error)
	Count(t string) (uint64, error)
	LatestDocs(limit int) ([]index.Document, error)
}

type highlightsRepository interface {
	Highlighted(userID int, doc index.Document) index.Document
}

type Config struct {
	LibraryPath   string
	CoverMaxWidth int
}

type Controller struct {
	hlRepository highlightsRepository
	idx          IdxReaderWriter
	sender       Sender
	config       Config
}

func NewController(hlRepository highlightsRepository, sender Sender, idx IdxReaderWriter, cfg Config) *Controller {
	return &Controller{
		hlRepository: hlRepository,
		idx:          idx,
		sender:       sender,
		config:       cfg,
	}
}
