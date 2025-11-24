package author

import (
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
)

type Sender interface {
	From() string
}

// IdxReader defines a set of author reading operations over an index
type IdxReader interface {
	SearchByAuthor(searchFields index.SearchFields, page, resultsPerPage int) (result.Paginated[[]index.Document], error)
	Author(slug, lang string) (index.Author, error)
	IndexAuthor(author index.Author) error
	Languages() ([]string, error)
}

type highlightsRepository interface {
	HighlightedPaginatedResult(userID int, results result.Paginated[[]index.Document]) result.Paginated[[]index.Document]
}

type readingRepository interface {
	CompletedPaginatedResult(userID int, results result.Paginated[[]index.Document]) result.Paginated[[]index.Document]
}

type Config struct {
	WordsPerMinute      float64
	CacheDir            string
	AuthorImageMaxWidth int
	ClientImageCacheTTL int
	ServerImageCacheTTL int
}

type Controller struct {
	hlRepository      highlightsRepository
	readingRepository readingRepository
	idx               IdxReader
	sender            Sender
	config            Config
	dataSource        DataSource
	appFs             afero.Fs
}

func NewController(hlRepository highlightsRepository, readingRepository readingRepository, sender Sender, idx IdxReader, cfg Config, dataSource DataSource, appFs afero.Fs) *Controller {
	return &Controller{
		hlRepository:      hlRepository,
		readingRepository: readingRepository,
		idx:               idx,
		sender:            sender,
		config:            cfg,
		dataSource:        dataSource,
		appFs:             appFs,
	}
}
