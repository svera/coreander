package document

import (
	"time"

	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/model"
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

type readingRepository interface {
	Get(userID int, documentPath string) (model.Reading, error)
	Update(userID int, documentPath, position string) error
	Touch(userID int, documentPath string) error
	RemoveDocument(documentPath string) error
	MarkComplete(userID int, documentPath string) error
	MarkIncomplete(userID int, documentPath string) error
	UpdateCompletionDate(userID int, documentPath string, completedAt time.Time) error
	Completed(userID int, doc index.Document) index.Document
	CompletedPaginatedResult(userID int, results result.Paginated[[]index.Document]) result.Paginated[[]index.Document]
}

type Config struct {
	WordsPerMinute        float64
	LibraryPath           string
	HomeDir               string
	CoverMaxWidth         int
	Hostname              string
	Port                  int
	UploadDocumentMaxSize int
	ClientImageCacheTTL   int
	ServerImageCacheTTL   int
}

type Controller struct {
	hlRepository      highlightsRepository
	readingRepository readingRepository
	idx               IdxReaderWriter
	sender            Sender
	config            Config
	metadataReaders   map[string]metadata.Reader
	appFs             afero.Fs
}

func NewController(hlRepository highlightsRepository, readingRepository readingRepository, sender Sender, idx IdxReaderWriter, metadataReaders map[string]metadata.Reader, appFs afero.Fs, cfg Config) *Controller {
	return &Controller{
		hlRepository:      hlRepository,
		readingRepository: readingRepository,
		idx:               idx,
		sender:            sender,
		config:            cfg,
		metadataReaders:   metadataReaders,
		appFs:             appFs,
	}
}
