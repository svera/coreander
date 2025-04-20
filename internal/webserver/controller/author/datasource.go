package author

import "github.com/svera/coreander/v4/internal/datasource/model"

type DataSource interface {
	SearchAuthor(name string, languages []string) (model.Author, error)
	RetrieveAuthor(IDs []string, languages []string) (model.Author, error)
}
