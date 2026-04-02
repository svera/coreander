package completed

import (
	"time"

	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

type idxReader interface {
	Document(slug string) (index.Document, error)
}

type readingRepository interface {
	CompletedPaginatedBetweenDates(userID int, startDate, endDate *time.Time, page int, resultsPerPage int, orderBy string) (result.Paginated[[]model.AugmentedDocument], error)
	CompletedStatsByYear(userID int, wordsPerMinute float64) ([]model.CompletedYearStats, error)
	Get(userID int, documentSlug string) (model.Reading, error)
	Touch(userID int, documentSlug string) error
	UpdateCompletionDate(userID int, documentSlug string, completedAt *time.Time) error
}

type Controller struct {
	readingRepository readingRepository
	idxReader         idxReader
}

func NewController(readingRepository readingRepository, idxReader idxReader) *Controller {
	return &Controller{
		readingRepository: readingRepository,
		idxReader:         idxReader,
	}
}
