package search

import (
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/result"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

const (
	TypeDocuments = "documents"
	TypeAuthors   = "authors"
)

type Sender interface {
	From() string
}

type IdxReader interface {
	Search(searchFields index.SearchFields, page, resultsPerPage int) (result.Paginated[[]index.Document], error)
	SearchAuthors(searchFields index.AuthorSearchFields, page, resultsPerPage int) (result.Paginated[[]index.Author], error)
	DocumentCountsByAuthorSlugs(slugs []string) (map[string]uint64, error)
	Count() (uint64, error)
	AuthorsCount() (uint64, error)
	Subjects() (map[string][]string, error)
}

type highlightsRepository interface {
	HighlightedPaginatedResult(userID int, results result.Paginated[[]model.AugmentedDocument]) result.Paginated[[]model.AugmentedDocument]
}

type readingRepository interface {
	CompletedPaginatedResult(userID int, results result.Paginated[[]model.AugmentedDocument]) result.Paginated[[]model.AugmentedDocument]
}

type Config struct {
	WordsPerMinute float64
}

type Controller struct {
	hlRepository      highlightsRepository
	readingRepository readingRepository
	idx               IdxReader
	sender            Sender
	config            Config
}

func NewController(hlRepository highlightsRepository, readingRepository readingRepository, sender Sender, idx IdxReader, cfg Config) *Controller {
	return &Controller{
		hlRepository:      hlRepository,
		readingRepository: readingRepository,
		idx:               idx,
		sender:            sender,
		config:            cfg,
	}
}
