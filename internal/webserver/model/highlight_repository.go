package model

import (
	"log"

	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HighlightRepository struct {
	DB *gorm.DB
}

type ShareDetail struct {
	Path             string
	SharedByName     string
	SharedByUsername string
	Comment          string
}

func (u *HighlightRepository) Highlights(userID int, page int, resultsPerPage int, sortBy, filter string) (result.Paginated[[]string], error) {
	highlights := []string{}
	var total int64

	query := u.DB.Table("highlights").Where("user_id = ?", userID)
	switch filter {
	case "highlights":
		query = query.Where("shared_by_id IS NULL")
	case "shared":
		query = query.Where("shared_by_id IS NOT NULL")
	}

	res := query.Scopes(Paginate(page, resultsPerPage)).Select("path").Order(sortBy).Pluck("path", &highlights)
	if res.Error != nil {
		log.Printf("error listing highlights: %s\n", res.Error)
		return result.Paginated[[]string]{}, res.Error
	}
	query.Count(&total)

	return result.NewPaginated(
		resultsPerPage,
		page,
		int(total),
		highlights,
	), nil
}

func (u *HighlightRepository) Total(userID int) (int, error) {
	var total int64
	res := u.DB.Table("highlights").Where("user_id = ?", userID).Count(&total)
	if res.Error != nil {
		log.Printf("error counting highlights: %s\n", res.Error)
		return 0, res.Error
	}
	return int(total), nil
}

func (u *HighlightRepository) ShareDetails(userID int, paths []string) (map[string]ShareDetail, error) {
	if len(paths) == 0 {
		return map[string]ShareDetail{}, nil
	}

	type shareRow struct {
		Path             string
		SharedByName     string `gorm:"column:shared_by_name"`
		SharedByUsername string `gorm:"column:shared_by_username"`
		Comment          string
	}

	rows := []shareRow{}
	res := u.DB.Table("highlights AS h").
		Select("h.path, u.name AS shared_by_name, u.username AS shared_by_username, h.comment").
		Joins("LEFT JOIN users u ON u.id = h.shared_by_id").
		Where("h.user_id = ? AND h.shared_by_id IS NOT NULL AND h.path IN (?)", userID, paths).
		Scan(&rows)
	if res.Error != nil {
		log.Printf("error listing shared highlights: %s\n", res.Error)
		return nil, res.Error
	}

	details := make(map[string]ShareDetail, len(rows))
	for _, row := range rows {
		details[row.Path] = ShareDetail{
			Path:             row.Path,
			SharedByName:     row.SharedByName,
			SharedByUsername: row.SharedByUsername,
			Comment:          row.Comment,
		}
	}

	return details, nil
}

func (u *HighlightRepository) HighlightedPaginatedResult(userID int, results result.Paginated[[]index.Document]) result.Paginated[[]index.Document] {
	highlights := make([]string, 0, len(results.Hits()))
	paths := make([]string, 0, len(results.Hits()))
	documents := make([]index.Document, len(results.Hits()))

	for _, path := range results.Hits() {
		paths = append(paths, path.ID)
	}
	u.DB.Table("highlights").Where(
		"user_id = ? AND path IN (?)",
		userID,
		paths,
	).Pluck("path", &highlights)

	highlightedPaths := make(map[string]struct{}, len(highlights))
	for _, path := range highlights {
		highlightedPaths[path] = struct{}{}
	}

	for i, doc := range results.Hits() {
		documents[i] = doc
		_, ok := highlightedPaths[doc.ID]
		documents[i].Highlighted = ok
	}

	return result.NewPaginated(
		ResultsPerPage,
		results.Page(),
		results.TotalHits(),
		documents,
	)
}

func (u *HighlightRepository) Highlighted(userID int, doc index.Document) index.Document {
	var count int64

	u.DB.Table("highlights").Where(
		"user_id = ? AND path = ?",
		userID,
		doc.ID,
	).Count(&count)

	if count == 1 {
		doc.Highlighted = true
	}
	return doc
}

func (u *HighlightRepository) Highlight(userID int, documentPath string) error {
	highlight := Highlight{
		UserID: userID,
		Path:   documentPath,
	}
	return u.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&highlight).Error
}

func (u *HighlightRepository) Remove(userID int, documentPath string) error {
	highlight := Highlight{
		UserID: userID,
		Path:   documentPath,
	}
	return u.DB.Delete(&highlight).Error
}

func (u *HighlightRepository) Share(senderID int, documentID, documentSlug, comment string, recipientIDs []int) error {
	if senderID <= 0 || documentID == "" || len(recipientIDs) == 0 {
		return nil
	}

	shares := make([]Highlight, 0, len(recipientIDs))
	sharedByID := senderID
	for _, recipientID := range recipientIDs {
		if recipientID <= 0 {
			continue
		}
		shares = append(shares, Highlight{
			UserID:     recipientID,
			Path:       documentID,
			SharedByID: &sharedByID,
			Comment:    comment,
		})
	}

	if len(shares) == 0 {
		return nil
	}

	// OnConflict{DoNothing: true} handles duplicates silently
	return u.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&shares).Error
}

func (u *HighlightRepository) RemoveDocument(documentPath string) error {
	return u.DB.Where("path = ?", documentPath).Delete(&Highlight{}).Error
}
