package completed

import (
	"time"

	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/result"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

const resultsPerPage = 12 // matches cover grid: 6 cols × 2 rows at xl

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
