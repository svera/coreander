package model

import (
	"gorm.io/gorm"
)

const (
	ResultsPerPage    = 10.0
	MaxPagesNavigator = 5
)

func Paginate(currentPage int, pageSize int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if currentPage == 0 {
			currentPage = 1
		}

		switch {
		case pageSize > 100:
			pageSize = 100
		case pageSize <= 0:
			pageSize = 10
		}

		offset := (currentPage - 1) * pageSize
		return db.Offset(offset).Limit(pageSize)
	}
}
