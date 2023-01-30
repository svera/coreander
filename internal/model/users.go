package model

import (
	"log"

	"gorm.io/gorm"
)

type Users struct {
	DB *gorm.DB
}

func (u *Users) List() ([]User, error) {
	users := []User{}
	result := u.DB.Find(&users)
	if result.Error != nil {
		log.Printf("error listing users: %s\n", result.Error)
	}
	return users, result.Error
}
