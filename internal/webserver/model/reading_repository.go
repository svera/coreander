package model

import (
	"log"
	"time"

	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReadingRepository struct {
	DB *gorm.DB
}

func (u *ReadingRepository) Latest(userID int, page int, resultsPerPage int) (result.Paginated[[]string], error) {
	docs := []string{}
	var total int64

	res := u.DB.Scopes(Paginate(page, resultsPerPage)).Table("readings").Select("path").Where("user_id = ?", userID).Order("updated_at DESC").Pluck("path", &docs)
	if res.Error != nil {
		log.Printf("error listing documents in progress: %s\n", res.Error)
		return result.Paginated[[]string]{}, res.Error
	}
	u.DB.Table("readings").Where("user_id = ?", userID).Count(&total)

	return result.NewPaginated(
		resultsPerPage,
		page,
		int(total),
		docs,
	), nil
}

func (u *ReadingRepository) Get(userID int, documentPath string) (Reading, error) {
	var reading Reading
	err := u.DB.Where("user_id = ? AND path = ?", userID, documentPath).First(&reading).Error
	return reading, err
}

func (u *ReadingRepository) Update(userID int, documentPath, position string) error {
	progress := Reading{
		UserID:   userID,
		Path:     documentPath,
		Position: position,
	}
	return u.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&progress).Error
}

// Touch creates a reading record if it doesn't exist, but doesn't update it if it does.
// This is used to track that a document has been opened without overwriting existing positions.
func (u *ReadingRepository) Touch(userID int, documentPath string) error {
	progress := Reading{
		UserID:   userID,
		Path:     documentPath,
		Position: "",
	}
	return u.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&progress).Error
}

func (u *ReadingRepository) RemoveDocument(documentPath string) error {
	return u.DB.Where("path = ?", documentPath).Delete(&Reading{}).Error
}

func (u *ReadingRepository) UpdateCompletionDate(userID int, documentPath string, completedAt *time.Time) error {
	return u.DB.Model(&Reading{}).
		Where("user_id = ? AND path = ?", userID, documentPath).
		UpdateColumn("completed_on", completedAt).Error
}

func (u *ReadingRepository) Completed(userID int, doc index.Document) index.Document {
	var reading Reading
	err := u.DB.Where("user_id = ? AND path = ? AND completed_on IS NOT NULL", userID, doc.ID).First(&reading).Error
	if err == nil && reading.CompletedOn != nil {
		doc.CompletedOn = reading.CompletedOn
	}
	return doc
}

func (u *ReadingRepository) CompletedPaginatedResult(userID int, results result.Paginated[[]index.Document]) result.Paginated[[]index.Document] {
	paths := make([]string, 0, len(results.Hits()))
	documents := make([]index.Document, len(results.Hits()))

	for _, doc := range results.Hits() {
		paths = append(paths, doc.ID)
	}

	var readings []Reading
	u.DB.Where(
		"user_id = ? AND path IN (?) AND completed_on IS NOT NULL",
		userID,
		paths,
	).Find(&readings)

	// Create a map for quick lookup
	readingMap := make(map[string]*time.Time)
	for _, r := range readings {
		if r.CompletedOn != nil {
			readingMap[r.Path] = r.CompletedOn
		}
	}

	for i, doc := range results.Hits() {
		documents[i] = doc
		if completedOn, exists := readingMap[doc.ID]; exists {
			documents[i].CompletedOn = completedOn
		}
	}

	return result.NewPaginated(
		ResultsPerPage,
		results.Page(),
		results.TotalHits(),
		documents,
	)
}

// CompletedBetweenDates returns paths of all readings completed by a user.
// If startDate and endDate are provided, it filters readings completed between those dates (inclusive).
// If startDate or endDate are nil, they are not used as filters.
func (u *ReadingRepository) CompletedBetweenDates(userID int, startDate, endDate *time.Time) ([]string, error) {
	var paths []string
	query := u.DB.Table("readings").Select("path").Where("user_id = ? AND completed_on IS NOT NULL", userID)

	if startDate != nil {
		query = query.Where("completed_on >= ?", startDate)
	}

	if endDate != nil {
		query = query.Where("completed_on <= ?", endDate)
	}

	err := query.Order("completed_on DESC").Pluck("path", &paths).Error

	if err != nil {
		log.Printf("error getting completed readings: %s\n", err)
		return nil, err
	}

	return paths, nil
}
