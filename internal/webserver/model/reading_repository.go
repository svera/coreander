package model

import (
	"log"
	"strconv"
	"strings"
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

// CompletedReadingsBetweenDates returns all completed readings for a user in the given date range (inclusive).
// Used when sorting by reading time requires fetching all readings then sorting in memory.
func (u *ReadingRepository) CompletedReadingsBetweenDates(userID int, startDate, endDate *time.Time) ([]Reading, error) {
	var readings []Reading
	query := u.DB.Table("readings").Where("user_id = ? AND completed_on IS NOT NULL", userID)
	if startDate != nil {
		query = query.Where("completed_on >= ?", startDate)
	}
	if endDate != nil {
		query = query.Where("completed_on <= ?", endDate)
	}
	err := query.Order("completed_on DESC").Find(&readings).Error
	if err != nil {
		log.Printf("error getting completed readings: %s\n", err)
		return nil, err
	}
	return readings, nil
}

// CompletedPaginated returns paginated completed readings for a user, ordered by completed_on (default DESC).
func (u *ReadingRepository) CompletedPaginated(userID int, page int, resultsPerPage int, orderBy string) (result.Paginated[[]Reading], error) {
	return u.CompletedPaginatedBetweenDates(userID, nil, nil, page, resultsPerPage, orderBy)
}

// CompletedPaginatedBetweenDates returns paginated completed readings for a user, optionally filtered by date range (inclusive).
// When startDate and endDate are both nil, all completed readings are returned.
// orderBy is e.g. "completed_on DESC" or "completed_on ASC"; if empty, "completed_on DESC" is used.
func (u *ReadingRepository) CompletedPaginatedBetweenDates(userID int, startDate, endDate *time.Time, page int, resultsPerPage int, orderBy string) (result.Paginated[[]Reading], error) {
	var readings []Reading
	var total int64

	if orderBy == "" {
		orderBy = "completed_on DESC"
	}

	baseQuery := u.DB.Table("readings").Where("user_id = ? AND completed_on IS NOT NULL", userID)
	if startDate != nil {
		baseQuery = baseQuery.Where("completed_on >= ?", startDate)
	}
	if endDate != nil {
		baseQuery = baseQuery.Where("completed_on <= ?", endDate)
	}

	if err := baseQuery.Count(&total).Error; err != nil {
		log.Printf("error counting completed readings: %s\n", err)
		return result.Paginated[[]Reading]{}, err
	}

	res := u.DB.Where("user_id = ? AND completed_on IS NOT NULL", userID)
	if startDate != nil {
		res = res.Where("completed_on >= ?", startDate)
	}
	if endDate != nil {
		res = res.Where("completed_on <= ?", endDate)
	}
	res = res.Order(orderBy).Scopes(Paginate(page, resultsPerPage)).Find(&readings)
	if res.Error != nil {
		log.Printf("error listing completed readings: %s\n", res.Error)
		return result.Paginated[[]Reading]{}, res.Error
	}

	return result.NewPaginated(
		resultsPerPage,
		page,
		int(total),
		readings,
	), nil
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

// completedStatsByYearRow is used to scan the raw SQL result.
type completedStatsByYearRow struct {
	Year    string
	DocCnt  int
	SlugsCS string
}

// CompletedStatsByYear returns a slice of years (sorted descending) with document count and slugs per year.
// Words are not set; the caller should use indexer.TotalWordCount(row.Slugs) to fill CompletedYearStats.Words.
func (u *ReadingRepository) CompletedStatsByYear(userID int) ([]CompletedYearStatsRow, error) {
	var rows []completedStatsByYearRow
	err := u.DB.Raw(
		`SELECT strftime('%Y', completed_on) AS year, COUNT(*) AS doc_cnt, group_concat(slug) AS slugs_cs
		 FROM readings
		 WHERE user_id = ? AND completed_on IS NOT NULL
		 GROUP BY strftime('%Y', completed_on)
		 ORDER BY year DESC`,
		userID,
	).Scan(&rows).Error
	if err != nil {
		log.Printf("error getting completed stats by year: %s\n", err)
		return nil, err
	}
	out := make([]CompletedYearStatsRow, 0, len(rows))
	for _, r := range rows {
		year, _ := strconv.Atoi(r.Year)
		slugs := []string{}
		if r.SlugsCS != "" {
			slugs = strings.Split(r.SlugsCS, ",")
		}
		out = append(out, CompletedYearStatsRow{
			Year:          year,
			DocumentCount: r.DocCnt,
			Slugs:         slugs,
		})
	}
	return out, nil
}
