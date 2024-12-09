package document

import (
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/result"
)

const relatedDocuments = 4

type Sender interface {
	SendDocument(address string, libraryPath string, fileName string) error
	From() string
}

// IdxReaderWriter defines a set of reading and writing operations over an index
type IdxReaderWriter interface {
	Search(keywords string, page, resultsPerPage int) (result.Paginated[[]index.Document], error)
	Count(t string) (uint64, error)
	Close() error
	Document(Slug string) (index.Document, error)
	SameSubjects(slug string, quantity int) ([]index.Document, error)
	SameAuthors(slug string, quantity int) ([]index.Document, error)
	SameSeries(slug string, quantity int) ([]index.Document, error)
	AddFile(file string) (string, error)
	RemoveFile(file string) error
	Documents(IDs []string) (map[string]index.Document, error)
}

type highlightsRepository interface {
	Highlights(userID int, page int, resultsPerPage int) (result.Paginated[[]string], error)
	Highlighted(userID int, doc index.Document) index.Document
	HighlightedPaginatedResult(userID int, results result.Paginated[[]index.Document]) result.Paginated[[]index.Document]
	RemoveDocument(documentPath string) error
}

type Config struct {
	WordsPerMinute        float64
	LibraryPath           string
	HomeDir               string
	CoverMaxWidth         int
	Hostname              string
	Port                  int
	UploadDocumentMaxSize int
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
