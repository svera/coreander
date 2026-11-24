package author

import (
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v5/internal/datasource/model"
)

type DataSource interface {
	AuthorDataSourceProfileProvider
	OwnsLink(key string) bool
	NormalizeAuthorID(value string) (string, error)
	SearchAuthor(name string, languages []string, firstPublicationDateUpperBound date.Date) (model.Author, error)
	RetrieveAuthor(IDs []string, languages []string, firstPublicationDateUpperBound date.Date) (model.Author, error)
}
