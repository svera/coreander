package highlight

import (
	"github.com/svera/coreander/v4/internal/model"
	"github.com/svera/coreander/v4/internal/search"
)

type highlightsRepository interface {
	Highlights(userID int, page int, resultsPerPage int) (search.PaginatedResult, error)
	Highlight(userID int, documentPath string) error
	Remove(userID int, documentPath string) error
	Highlighted(userID int, documents []search.Document) []search.Document
}

// IdxReaderWriter defines a set of reading and writing operations over an index
type IdxReaderWriter interface {
	Search(keywords string, page, resultsPerPage int) (*search.PaginatedResult, error)
	Count() (uint64, error)
	Close() error
	Document(Slug string) (search.Document, error)
	Documents(IDs []string) ([]search.Document, error)
	SameSubjects(slug string, quantity int) ([]search.Document, error)
	SameAuthors(slug string, quantity int) ([]search.Document, error)
	SameSeries(slug string, quantity int) ([]search.Document, error)
	RemoveFile(file string) error
}

type usersRepository interface {
	FindByUuid(uuid string) (*model.User, error)
}

type Sender interface {
	SendDocument(address string, libraryPath string, fileName string) error
	From() string
}

type Controller struct {
	hlRepository   highlightsRepository
	usrRepository  usersRepository
	idx            IdxReaderWriter
	sender         Sender
	wordsPerMinute float64
}

func NewController(hlRepository highlightsRepository, usrRepository usersRepository, sender Sender, wordsPerMinute float64, idx IdxReaderWriter) *Controller {
	return &Controller{
		hlRepository:   hlRepository,
		usrRepository:  usrRepository,
		idx:            idx,
		sender:         sender,
		wordsPerMinute: wordsPerMinute,
	}
}
