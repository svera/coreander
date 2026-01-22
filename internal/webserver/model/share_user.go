package model

import "time"

type ShareUser struct {
	CreatedAt        time.Time `gorm:"autoCreateTime"`
	ShareID          uint `gorm:"index; not null"`
	UserID           int  `gorm:"index; not null"`
}

func (ShareUser) TableName() string {
	return "shares_users"
}
