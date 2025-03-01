package author

import (
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
)

type Sender interface {
	From() string
}

// IdxReader defines a set of author reading operations over an index
type IdxReader interface {
	SearchByAuthor(authorSlug string, page, resultsPerPage int) (result.Paginated[[]index.Document], error)
	Author(slug, lang string) (index.Author, error)
	IndexAuthor(author index.Author) error
}

type highlightsRepository interface {
	HighlightedPaginatedResult(userID int, results result.Paginated[[]index.Document]) result.Paginated[[]index.Document]
}

type Config struct {
	WordsPerMinute float64
}

type Controller struct {
	hlRepository highlightsRepository
	idx          IdxReader
	sender       Sender
	config       Config
	dataSource   DataSource
}

func NewController(hlRepository highlightsRepository, sender Sender, idx IdxReader, cfg Config, dataSource DataSource) *Controller {
	return &Controller{
		hlRepository: hlRepository,
		idx:          idx,
		sender:       sender,
		config:       cfg,
		dataSource:   dataSource,
	}
}
