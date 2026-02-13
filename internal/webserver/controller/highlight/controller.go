package highlight

import (
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

const latestHighlightsAmount = 6

type highlightsRepository interface {
	Highlights(userID int, page int, resultsPerPage int, sortBy, filter string) (result.Paginated[[]model.Highlight], error)
	Total(userID int) (int, error)
	Highlight(userID int, documentPath string) error
	Remove(userID int, documentPath string) error
	Highlighted(userID int, documents model.AugmentedDocument) model.AugmentedDocument
}

type readingRepository interface {
	CompletedPaginatedResult(userID int, results result.Paginated[[]model.AugmentedDocument]) result.Paginated[[]model.AugmentedDocument]
}

// IdxReaderWriter defines a set of reading and writing operations over an index
type IdxReaderWriter interface {
	Document(Slug string) (index.Document, error)
	DocumentByID(ID string) (index.Document, error)
	Languages() ([]string, error)
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
	hlRepository      highlightsRepository
	readingRepository readingRepository
	usrRepository     usersRepository
	idx               IdxReaderWriter
	sender            Sender
	wordsPerMinute    float64
}

func NewController(hlRepository highlightsRepository, readingRepository readingRepository, usrRepository usersRepository, sender Sender, wordsPerMinute float64, idx IdxReaderWriter) *Controller {
	return &Controller{
		hlRepository:      hlRepository,
		readingRepository: readingRepository,
		usrRepository:     usrRepository,
		idx:               idx,
		sender:            sender,
		wordsPerMinute:    wordsPerMinute,
	}
}
