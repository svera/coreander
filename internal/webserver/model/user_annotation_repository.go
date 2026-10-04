package model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserAnnotationRepository struct {
	DB *gorm.DB
}

func (r *UserAnnotationRepository) Save(userID int, slug, cfi, content string) error {
	return r.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "slug"}, {Name: "cfi"}},
		DoUpdates: clause.AssignmentColumns([]string{"content", "updated_at"}),
	}).Create(&UserAnnotation{
		UserID: userID, Slug: slug, CFI: cfi, Content: content,
	}).Error
}

func (r *UserAnnotationRepository) List(userID int, slug string) ([]UserAnnotation, error) {
	annotations := []UserAnnotation{}
	err := r.DB.Where("user_id = ? AND slug = ?", userID, slug).
		Order("created_at ASC").Find(&annotations).Error
	return annotations, err
}

func (r *UserAnnotationRepository) RemoveDocument(slug string) error {
	return r.DB.Where("slug = ?", slug).Delete(&UserAnnotation{}).Error
}

func (r *UserAnnotationRepository) Delete(userID int, slug, cfi string) error {
	return r.DB.Where("user_id = ? AND slug = ? AND cfi = ?", userID, slug, cfi).
		Delete(&UserAnnotation{}).Error
}
