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
	Uuid        string `gorm:"uniqueIndex"`
	Name        string
	Email       string `gorm:"uniqueIndex"`
	SendToEmail string
	Password    string
	Role        int
}

type Users struct {
	DB *gorm.DB
}

func (u *Users) List(page int, resultsPerPage int) ([]User, error) {
	users := []User{}
	result := u.DB.Scopes(Paginate(page, resultsPerPage)).Order("email ASC").Find(&users)
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

func (u *Users) Update(user User) error {
	if result := u.DB.Save(&user); result.Error != nil {
		log.Printf("error updating user: %s\n", result.Error)
		return result.Error
	}
	return nil
}

func (u *Users) Exist(email string) bool {
	user := User{}
	return u.DB.Where("email = ?", email).First(&user).RowsAffected == 1
}

func (u *Users) Admins() int64 {
	var totalRows int64
	u.DB.Where("role = ?", RoleAdmin).Take(&[]User{}).Count(&totalRows)
	return totalRows
}

func (u *Users) Delete(uuid string) error {
	user, err := u.Find(uuid)
	if err != nil {
		return nil
	}
	if u.Admins() == 1 && user.Role == RoleAdmin {
		return nil
	}
	result := u.DB.Where("uuid = ?", uuid).Delete(&user)
	if result.Error != nil {
		log.Printf("error deleting user: %s\n", result.Error)
	}
	return nil
}

func (u *Users) CheckCredentials(email, password string) (User, error) {
	var user User

	result := u.DB.Where("email = ? AND password = ?", email, Hash(password)).Take(&user)
	return user, result.Error
}
