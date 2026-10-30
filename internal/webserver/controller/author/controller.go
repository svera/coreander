package author

import (
	"io/fs"

	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/result"
	"github.com/svera/coreander/v5/internal/servicelinks"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

type Sender interface {
	From() string
}

type LinkProvider interface {
	Author(author index.Author) servicelinks.Link
}

// IdxReader defines a set of author reading operations over an index
type IdxReader interface {
	SearchAuthors(searchFields index.AuthorSearchFields, page, resultsPerPage int) (result.Paginated[[]index.Author], error)
	SearchByAuthor(searchFields index.SearchFields, page, resultsPerPage int) (result.Paginated[[]index.Document], error)
	Author(slug, lang string) (index.Author, error)
	IndexAuthor(author index.Author) error
	Languages() ([]string, error)
}

type highlightsRepository interface {
	HighlightedPaginatedResult(userID int, results result.Paginated[[]model.AugmentedDocument]) result.Paginated[[]model.AugmentedDocument]
}

type readingRepository interface {
	CompletedPaginatedResult(userID int, results result.Paginated[[]model.AugmentedDocument]) result.Paginated[[]model.AugmentedDocument]
}

type Config struct {
	WordsPerMinute      float64
	CacheDir            string
	CacheMaxSize        int
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
	embeddedImagesFS  fs.FS
	linkProvider      LinkProvider
}

func NewController(hlRepository highlightsRepository, readingRepository readingRepository, sender Sender, idx IdxReader, cfg Config, dataSource DataSource, appFs afero.Fs, embeddedImagesFS fs.FS, linkProvider LinkProvider) *Controller {
	return &Controller{
		hlRepository:      hlRepository,
		readingRepository: readingRepository,
		idx:               idx,
		sender:            sender,
		config:            cfg,
		dataSource:        dataSource,
		appFs:             appFs,
		embeddedImagesFS:  embeddedImagesFS,
		linkProvider:      linkProvider,
	}
}

func (a *Controller) authorLink(author index.Author) servicelinks.Link {
	if a.linkProvider == nil {
		return servicelinks.Link{}
	}
	return a.linkProvider.Author(author)
}
