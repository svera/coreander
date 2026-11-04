package document

import (
	"context"
	"image"
	"time"

	"github.com/spf13/afero"
	datasourcemodel "github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/i18n"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/result"
	authorcontroller "github.com/svera/coreander/v5/internal/webserver/controller/author"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

const relatedDocuments = 4

type ReviewProvider interface {
	Name() string
	Score(ctx context.Context, document index.Document) (datasourcemodel.Score, error)
	// Reviews returns reviews in the current UI language (a two-letter ISO code).
	Reviews(ctx context.Context, document index.Document, language string) (datasourcemodel.ReviewPage, error)
	// CanEditDocumentLink reports whether this provider supports correcting its document link.
	CanEditDocumentLink() bool
	// UpdateDocumentLink saves an administrator's correction to the provider-specific link.
	UpdateDocumentLink(document index.Document, link string) error
}

type Sender interface {
	SendBCC(addresses []string, subject, body string) error
	SendDocument(address, subject string, file []byte, fileName string) error
	From() string
}

// IdxReaderWriter defines a set of reading and writing operations over an index
type IdxReaderWriter interface {
	Search(searchFields index.SearchFields, page, resultsPerPage int) (result.CappedPaginatedResult[[]index.Document], error)
	TotalDocs() (uint64, error)
	Close() error
	Document(Slug string) (index.Document, error)
	File(slug, ifNoneMatch string) (*index.IndexedFile, error)
	Cover(slug string, coverMaxWidth int) (image.Image, error)
	SameAuthors(slug string, quantity int) ([]index.Document, error)
	SameSeries(slug string, quantity int) (result.Paginated[[]index.Document], error)
	NewFile(fileName string, contents []byte) (string, error)
	DeleteDocument(slug string) error
	Documents(slugs []string) (map[string]index.Document, error)
	Languages() ([]string, error)
	Subjects() (map[string][]string, error)
	Author(slug, lang string) (index.Author, error)
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
	Update(userID int, documentSlug, position string, percentage *int) error
	Touch(userID int, documentSlug string) error
	RemoveDocument(documentSlug string) error
	UpdateCompletionDate(userID int, documentSlug string, completedAt *time.Time) error
	CompletedOn(userID int, documentSlug string) (*time.Time, error)
	CompletedPaginatedResult(userID int, results result.Paginated[[]model.AugmentedDocument]) result.Paginated[[]model.AugmentedDocument]
}

type annotationsRepository interface {
	RemoveDocument(slug string) error
}

type Config struct {
	WordsPerMinute        float64
	HomeDir               string
	CoverMaxWidth         int
	CacheDir              string
	CacheMaxSize          int
	Hostname              string
	Port                  int
	UploadDocumentMaxSize int
	ClientImageCacheTTL   int
	ServerImageCacheTTL   int
	ShareCommentMaxSize   int
	ShareMaxRecipients    int
}

type Controller struct {
	hlRepository          highlightsRepository
	usersRepository       usersRepository
	readingRepository     readingRepository
	annotationsRepository annotationsRepository
	idx                   IdxReaderWriter
	sender                Sender
	config                Config
	appFs                 afero.Fs
	translator            i18n.Translator
	reviewProvider        ReviewProvider
	linkProviders         []authorcontroller.DataSourceLinkProvider
}

// NewController creates a document controller. A nil reviewProvider disables scores and reviews.
func NewController(hlRepository highlightsRepository, usersRepository usersRepository, readingRepository readingRepository, annotationsRepository annotationsRepository, sender Sender, idx IdxReaderWriter, appFs afero.Fs, cfg Config, translator i18n.Translator, reviewProvider ReviewProvider, linkProviders ...authorcontroller.DataSourceLinkProvider) *Controller {
	return &Controller{
		hlRepository:          hlRepository,
		usersRepository:       usersRepository,
		readingRepository:     readingRepository,
		annotationsRepository: annotationsRepository,
		idx:                   idx,
		sender:                sender,
		config:                cfg,
		appFs:                 appFs,
		translator:            translator,
		reviewProvider:        reviewProvider,
		linkProviders:         linkProviders,
	}
}

// DataSourceName returns the source name used for review-score labels.
func (d *Controller) DataSourceName() string {
	if d.reviewProvider == nil {
		return ""
	}
	return d.reviewProvider.Name()
}
