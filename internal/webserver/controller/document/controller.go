package document

import (
	"github.com/spf13/afero"
	"github.com/svera/coreander/v3/internal/metadata"
	"github.com/svera/coreander/v3/internal/result"
)

const relatedDocuments = 4

type Sender interface {
	SendDocument(address string, libraryPath string, fileName string) error
	From() string
}

// IdxReaderWriter defines a set of reading and writing operations over an index
type IdxReaderWriter interface {
	Search(keywords string, page, resultsPerPage int) (result.Paginated[[]metadata.Document], error)
	Count() (uint64, error)
	Close() error
	Document(Slug string) (metadata.Document, error)
	SameSubjects(slug string, quantity int) ([]metadata.Document, error)
	SameAuthors(slug string, quantity int) ([]metadata.Document, error)
	SameSeries(slug string, quantity int) ([]metadata.Document, error)
	RemoveFile(file string) error
}

type highlightsRepository interface {
	Highlighted(userID int, doc metadata.Document) metadata.Document
	HighlightedPaginatedResult(userID int, results result.Paginated[[]metadata.Document]) result.Paginated[[]metadata.Document]
}

type Config struct {
	WordsPerMinute float64
	LibraryPath    string
	HomeDir        string
	CoverMaxWidth  int
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
