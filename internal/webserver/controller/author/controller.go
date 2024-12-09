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
	Author(slug string) (index.Author, error)
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
}

func NewController(hlRepository highlightsRepository, sender Sender, idx IdxReader, cfg Config) *Controller {
	return &Controller{
		hlRepository: hlRepository,
		idx:          idx,
		sender:       sender,
		config:       cfg,
	}
}
