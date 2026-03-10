package model

import (
	"log"
	"strconv"
	"time"

	"github.com/svera/coreander/v4/internal/result"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReadingRepository struct {
	DB *gorm.DB
}

func (u *ReadingRepository) Latest(userID int, page int, resultsPerPage int) (result.Paginated[[]string], error) {
	slugs := []string{}
	var total int64

	res := u.DB.Scopes(Paginate(page, resultsPerPage)).Table("readings").Select("slug").Where("user_id = ? AND completed_on IS NULL", userID).Order("updated_at DESC").Pluck("slug", &slugs)
	if res.Error != nil {
		log.Printf("error listing documents in progress: %s\n", res.Error)
		return result.Paginated[[]string]{}, res.Error
	}
	u.DB.Table("readings").Where("user_id = ? AND completed_on IS NULL", userID).Count(&total)

	return result.NewPaginated(
		resultsPerPage,
		page,
		int(total),
		slugs,
	), nil
}

func (u *ReadingRepository) Get(userID int, documentSlug string) (Reading, error) {
	var reading Reading
	err := u.DB.Where("user_id = ? AND slug = ?", userID, documentSlug).First(&reading).Error
	return reading, err
}

func (u *ReadingRepository) Update(userID int, documentSlug, position string) error {
	progress := Reading{
		UserID:   userID,
		Slug:     documentSlug,
		Position: position,
	}
	return u.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&progress).Error
}

// Touch creates a reading record if it doesn't exist, but doesn't update it if it does.
// This is used to track that a document has been opened without overwriting existing positions.
// Sets updated_at to NULL initially - it will only be set when the reading position is actually updated.
func (u *ReadingRepository) Touch(userID int, documentSlug string) error {
	// Check if record already exists
	var count int64
	u.DB.Model(&Reading{}).Where("user_id = ? AND slug = ?", userID, documentSlug).Count(&count)
	if count > 0 {
		return nil // Record already exists, do nothing
	}

	progress := Reading{
		UserID: userID,
		Slug:   documentSlug,
	}
	return u.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&progress).Error
}

func (u *ReadingRepository) RemoveDocument(documentSlug string) error {
	return u.DB.Where("slug = ?", documentSlug).Delete(&Reading{}).Error
}

func (u *ReadingRepository) UpdateCompletionDate(userID int, documentSlug string, completedAt *time.Time) error {
	return u.DB.Model(&Reading{}).
		Where("user_id = ? AND slug = ?", userID, documentSlug).
		UpdateColumn("completed_on", completedAt).Error
}

func (u *ReadingRepository) CompletedOn(userID int, documentSlug string) (*time.Time, error) {
	var reading Reading
	err := u.DB.Where("user_id = ? AND slug = ? AND completed_on IS NOT NULL", userID, documentSlug).First(&reading).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return reading.CompletedOn, nil
}

func (u *ReadingRepository) CompletedPaginatedResult(userID int, results result.Paginated[[]AugmentedDocument]) result.Paginated[[]AugmentedDocument] {
	slugs := make([]string, 0, len(results.Hits()))
	searchResults := make([]AugmentedDocument, len(results.Hits()))

	for _, searchResult := range results.Hits() {
		slugs = append(slugs, searchResult.Slug)
	}

	var readings []Reading
	u.DB.Where(
		"user_id = ? AND slug IN (?) AND completed_on IS NOT NULL",
		userID,
		slugs,
	).Find(&readings)

	// Create a map for quick lookup
	readingMap := make(map[string]*time.Time)
	for _, r := range readings {
		if r.CompletedOn != nil {
			readingMap[r.Slug] = r.CompletedOn
		}
	}

	for i, searchResult := range results.Hits() {
		if completedOn, exists := readingMap[searchResult.Slug]; exists {
			searchResult.CompletedOn = completedOn
		}
		searchResults[i] = searchResult
	}

	return result.NewPaginated(
		ResultsPerPage,
		results.Page(),
		results.TotalHits(),
		searchResults,
	)
}

// CompletedBetweenDates returns slugs of all readings completed by a user.
// If startDate and endDate are provided, it filters readings completed between those dates (inclusive).
// If startDate or endDate are nil, they are not used as filters.
func (u *ReadingRepository) CompletedBetweenDates(userID int, startDate, endDate *time.Time) ([]string, error) {
	var slugs []string
	query := u.DB.Table("readings").Select("slug").Where("user_id = ? AND completed_on IS NOT NULL", userID)

	if startDate != nil {
		query = query.Where("completed_on >= ?", startDate)
	}

	if endDate != nil {
		query = query.Where("completed_on <= ?", endDate)
	}

	err := query.Order("completed_on DESC").Pluck("slug", &slugs).Error

	if err != nil {
		log.Printf("error getting completed readings: %s\n", err)
		return nil, err
	}

	return slugs, nil
}

// CompletedYears returns the years with completed readings for a user.
func (u *ReadingRepository) CompletedYears(userID uint) ([]int, error) {
	var yearStrings []string
	err := u.DB.Raw(
		"SELECT DISTINCT strftime('%Y', completed_on) AS year FROM readings WHERE user_id = ? AND completed_on IS NOT NULL AND strftime('%Y', completed_on) <> strftime('%Y', 'now') ORDER BY year DESC",
		userID,
	).Scan(&yearStrings).Error
	if err != nil {
		log.Printf("error getting completed years: %s\n", err)
		return nil, err
	}

	years := make([]int, 0, len(yearStrings))
	for _, yearString := range yearStrings {
		if year, convErr := strconv.Atoi(yearString); convErr == nil {
			years = append(years, year)
		}
	}

	return years, nil
}
