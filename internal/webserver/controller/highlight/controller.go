package highlight

import (
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

const latestHighlightsAmount = 6

type highlightsRepository interface {
	Highlights(userID int, page int, resultsPerPage int, sortBy string) (result.Paginated[[]string], error)
	Highlight(userID int, documentPath string) error
	Remove(userID int, documentPath string) error
	Highlighted(userID int, documents index.Document) index.Document
}

// IdxReaderWriter defines a set of reading and writing operations over an index
type IdxReaderWriter interface {
	Document(Slug string) (index.Document, error)
	DocumentByID(ID string) (index.Document, error)
}

type usersRepository interface {
	FindByUuid(uuid string) (*model.User, error)
	FindByUsername(username string) (*model.User, error)
}

type Sender interface {
	SendDocument(address, subject, libraryPath, fileName string) error
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
