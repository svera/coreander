package model

import (
	"log"

	"gorm.io/gorm"
)

const (
	RoleRegular = 1
	RoleAdmin   = 2
)

type User struct {
	gorm.Model
	Uuid     string `gorm:"uniqueIndex"`
	Name     string
	Username string `gorm:"uniqueIndex"`
	Password string
	Role     float64
}

type Users struct {
	DB *gorm.DB
}

func (u *Users) List(page int, resultsPerPage int) ([]User, error) {
	users := []User{}
	result := u.DB.Scopes(Paginate(page, resultsPerPage)).Order("username ASC").Find(&users)
	if result.Error != nil {
		log.Printf("error listing users: %s\n", result.Error)
	}
	return users, result.Error
}

func (u *Users) Total() int64 {
	var totalRows int64
	users := []User{}
	u.DB.Model(&users).Count(&totalRows)
	return totalRows
}

func (u *Users) Find(uuid string) (User, error) {
	user := User{}
	result := u.DB.Where("uuid = ?", uuid).Take(&user)
	if result.Error != nil {
		log.Printf("error retrieving user: %s\n", result.Error)
	}
	return user, result.Error
}

func (u *Users) Create(user User) error {
	if result := u.DB.Create(&user); result.Error != nil {
		log.Printf("error creating user: %s\n", result.Error)
		return result.Error
	}
	return nil
}

func (u *Users) Exist(username string) bool {
	user := User{}
	return u.DB.Where("username = ?", username).First(&user).RowsAffected == 1
}
