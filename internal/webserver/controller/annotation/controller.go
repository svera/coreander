package annotation

import (
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

type repository interface {
	Save(userID int, slug, cfi, content, comment string) error
	List(userID int, slug string) ([]model.Annotation, error)
	Delete(userID int, slug, cfi string) error
}

type indexReader interface {
	Document(slug string) (index.Document, error)
}

type Controller struct {
	repository repository
	idx        indexReader
}

func NewController(repository repository, idx indexReader) *Controller {
	return &Controller{repository: repository, idx: idx}
}
