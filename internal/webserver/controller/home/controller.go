package home

import (
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
)

type Sender interface {
	SendDocument(address string, libraryPath string, fileName string) error
	From() string
}

// IdxReaderWriter defines a set of reading and writing operations over an index
type IdxReaderWriter interface {
	Documents(IDs []string) (map[string]index.Document, error)
	Count() (uint64, error)
}

type highlightsRepository interface {
	Highlights(userID int, page int, resultsPerPage int) (result.Paginated[[]string], error)
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
