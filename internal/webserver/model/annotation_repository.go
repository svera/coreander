package model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AnnotationRepository struct {
	DB *gorm.DB
}

func (r *AnnotationRepository) Save(userID int, slug, cfi, content, comment string) error {
	return r.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "slug"}, {Name: "cfi"}},
		DoUpdates: clause.AssignmentColumns([]string{"content", "comment", "updated_at"}),
	}).Create(&Annotation{
		UserID: userID, Slug: slug, CFI: cfi, Content: content, Comment: comment,
	}).Error
}

func (r *AnnotationRepository) List(userID int, slug string) ([]Annotation, error) {
	annotations := []Annotation{}
	err := r.DB.Where("user_id = ? AND slug = ?", userID, slug).
		Order("created_at ASC").Find(&annotations).Error
	return annotations, err
}

func (r *AnnotationRepository) RemoveDocument(slug string) error {
	return r.DB.Where("slug = ?", slug).Delete(&Annotation{}).Error
}

func (r *AnnotationRepository) Delete(userID int, slug, cfi string) error {
	return r.DB.Where("user_id = ? AND slug = ? AND cfi = ?", userID, slug, cfi).
		Delete(&Annotation{}).Error
}
