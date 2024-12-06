package author

import (
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/result"
)

type Sender interface {
	From() string
}

// IdxReaderWriter defines a set of reading and writing operations over an index
type IdxReaderWriter interface {
	SearchByAuthor(authorSlug string, page, resultsPerPage int) (result.Paginated[[]index.Document], error)
	Author(slug string) (index.Author, error)
	Count(t string) (uint64, error)
	Close() error
	Documents(IDs []string) (map[string]index.Document, error)
}

type highlightsRepository interface {
	HighlightedPaginatedResult(userID int, results result.Paginated[[]index.Document]) result.Paginated[[]index.Document]
}

type Config struct {
	WordsPerMinute float64
	LibraryPath    string
	HomeDir        string
	CoverMaxWidth  int
	Hostname       string
	Port           int
}

type Controller struct {
	hlRepository    highlightsRepository
	idx             IdxReaderWriter
	sender          Sender
	config          Config
	metadataReaders map[string]metadata.Reader
	appFs           afero.Fs
}

func NewController(hlRepository highlightsRepository, sender Sender, idx IdxReaderWriter, metadataReaders map[string]metadata.Reader, appFs afero.Fs, cfg Config) *Controller {
	return &Controller{
		hlRepository:    hlRepository,
		idx:             idx,
		sender:          sender,
		config:          cfg,
		metadataReaders: metadataReaders,
		appFs:           appFs,
	}
}
