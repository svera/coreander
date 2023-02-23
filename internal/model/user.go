package model

import (
	"gorm.io/gorm"
)

const (
	RoleRegular = 1
	RoleAdmin   = 2
)

type User struct {
	gorm.Model
	Uuid        string `gorm:"uniqueIndex"`
	Name        string
	Email       string `gorm:"uniqueIndex"`
	SendToEmail string
	Password    string
	Role        int
}
