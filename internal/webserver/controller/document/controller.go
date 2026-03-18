package document

import (
	"time"

	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/i18n"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

const relatedDocuments = 4

type Sender interface {
	SendBCC(addresses []string, subject, body string) error
	SendDocument(address, subject string, file []byte, fileName string) error
	From() string
}

// IdxReaderWriter defines a set of reading and writing operations over an index
type IdxReaderWriter interface {
	Search(searchFields index.SearchFields, page, resultsPerPage int) (result.Paginated[[]index.Document], error)
	Count() (uint64, error)
	Close() error
	Document(Slug string) (index.Document, error)
	File(slug string) (*index.IndexedFile, error)
	Cover(slug string, coverMaxWidth int) ([]byte, error)
	SameSubjects(slug string, quantity int) ([]index.Document, error)
	SameAuthors(slug string, quantity int) ([]index.Document, error)
	SameSeries(slug string, quantity int) ([]index.Document, error)
	NewFile(fileName string, contents []byte) (string, error)
	DeleteDocument(slug string) error
	Documents(slugs []string) (map[string]index.Document, error)
	Languages() ([]string, error)
	Subjects() (map[string][]string, error)
}

type highlightsRepository interface {
	Highlighted(userID int, doc model.AugmentedDocument) model.AugmentedDocument
	HighlightedPaginatedResult(userID int, results result.Paginated[[]model.AugmentedDocument]) result.Paginated[[]model.AugmentedDocument]
	RemoveDocument(documentSlug string) error
	Share(senderID int, documentSlug, comment string, recipientIDs []int) error
}

type usersRepository interface {
	FindByEmail(email string) (*model.User, error)
	FindByUsername(username string) (*model.User, error)
}

type readingRepository interface {
	Get(userID int, documentSlug string) (model.Reading, error)
	Update(userID int, documentSlug, position string) error
	Touch(userID int, documentSlug string) error
	RemoveDocument(documentSlug string) error
	UpdateCompletionDate(userID int, documentSlug string, completedAt *time.Time) error
	CompletedOn(userID int, documentSlug string) (*time.Time, error)
	CompletedPaginatedResult(userID int, results result.Paginated[[]model.AugmentedDocument]) result.Paginated[[]model.AugmentedDocument]
}

type Config struct {
	WordsPerMinute        float64
	HomeDir               string
	CoverMaxWidth         int
	Hostname              string
	Port                  int
	UploadDocumentMaxSize int
	ClientImageCacheTTL   int
	ServerImageCacheTTL   int
	ShareCommentMaxSize   int
	ShareMaxRecipients    int
}

type Controller struct {
	hlRepository      highlightsRepository
	usersRepository   usersRepository
	readingRepository readingRepository
	idx               IdxReaderWriter
	sender            Sender
	config            Config
	metadataReaders   map[string]metadata.Reader
	appFs             afero.Fs
	translator        i18n.Translator
}

func NewController(hlRepository highlightsRepository, usersRepository usersRepository, readingRepository readingRepository, sender Sender, idx IdxReaderWriter, metadataReaders map[string]metadata.Reader, appFs afero.Fs, cfg Config, translator i18n.Translator) *Controller {
	return &Controller{
		hlRepository:      hlRepository,
		usersRepository:   usersRepository,
		readingRepository: readingRepository,
		idx:               idx,
		sender:            sender,
		config:            cfg,
		metadataReaders:   metadataReaders,
		appFs:             appFs,
		translator:        translator,
	}
}
