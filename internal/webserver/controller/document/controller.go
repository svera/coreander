package document

import (
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/result"
)

const relatedDocuments = 4

type Sender interface {
	SendDocument(address, subject, libraryPath, fileName string) error
	From() string
}

// IdxReaderWriter defines a set of reading and writing operations over an index
type IdxReaderWriter interface {
	Search(searchFields index.SearchFields, page, resultsPerPage int) (result.Paginated[[]index.Document], error)
	Count(t string) (uint64, error)
	Close() error
	Document(Slug string) (index.Document, error)
	SameSubjects(slug string, quantity int) ([]index.Document, error)
	SameAuthors(slug string, quantity int) ([]index.Document, error)
	SameSeries(slug string, quantity int) ([]index.Document, error)
	AddFile(file string) (string, error)
	RemoveFile(file string) error
	Documents(IDs []string, sortBy []string) ([]index.Document, error)
}

type highlightsRepository interface {
	Highlights(userID int, page int, resultsPerPage int, sortBy string) (result.Paginated[[]string], error)
	Highlighted(userID int, doc index.Document) index.Document
	HighlightedPaginatedResult(userID int, results result.Paginated[[]index.Document]) result.Paginated[[]index.Document]
	RemoveDocument(documentPath string) error
}

type historyRepository interface {
	UpdateReading(userID int, documentPath string) error
	Remove(documentPath string) error
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
	hlRepository      highlightsRepository
	historyRepository historyRepository
	idx               IdxReaderWriter
	sender            Sender
	config            Config
	metadataReaders   map[string]metadata.Reader
	appFs             afero.Fs
}

func NewController(hlRepository highlightsRepository, historyRepository historyRepository, sender Sender, idx IdxReaderWriter, metadataReaders map[string]metadata.Reader, appFs afero.Fs, cfg Config) *Controller {
	return &Controller{
		hlRepository:      hlRepository,
		historyRepository: historyRepository,
		idx:               idx,
		sender:            sender,
		config:            cfg,
		metadataReaders:   metadataReaders,
		appFs:             appFs,
	}
}
