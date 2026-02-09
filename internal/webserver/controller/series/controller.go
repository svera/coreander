package series

import (
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

type Sender interface {
	From() string
}

// IdxReader defines a set of author reading operations over an index
type IdxReader interface {
	SearchBySeries(searchFields index.SearchFields, page, resultsPerPage int) (result.Paginated[[]index.Document], error)
	Languages() ([]string, error)
}

type highlightsRepository interface {
	HighlightedPaginatedResult(userID int, results result.Paginated[[]model.SearchResult]) result.Paginated[[]model.SearchResult]
}

type readingRepository interface {
	CompletedPaginatedResult(userID int, results result.Paginated[[]model.SearchResult]) result.Paginated[[]model.SearchResult]
}

type Config struct {
	WordsPerMinute float64
}

type Controller struct {
	hlRepository      highlightsRepository
	readingRepository readingRepository
	idx               IdxReader
	sender            Sender
	config            Config
	appFs             afero.Fs
}

func NewController(hlRepository highlightsRepository, readingRepository readingRepository, sender Sender, idx IdxReader, cfg Config, appFs afero.Fs) *Controller {
	return &Controller{
		hlRepository:      hlRepository,
		readingRepository: readingRepository,
		idx:               idx,
		sender:            sender,
		config:            cfg,
		appFs:             appFs,
	}
}
