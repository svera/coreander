package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log"

	"gorm.io/gorm"
)

type UserRepository struct {
	DB *gorm.DB
}

func (u *UserRepository) List(page int, resultsPerPage int) ([]User, error) {
	users := []User{}
	result := u.DB.Scopes(Paginate(page, resultsPerPage)).Order("email ASC").Find(&users)
	if result.Error != nil {
		log.Printf("error listing users: %s\n", result.Error)
	}
	return users, result.Error
}

func (u *UserRepository) Total() int64 {
	var totalRows int64
	users := []User{}
	u.DB.Model(&users).Count(&totalRows)
	return totalRows
}

func (u *UserRepository) FindByUuid(uuid string) (User, error) {
	return u.find("uuid", uuid)
}

func (u *UserRepository) Create(user User) error {
	if result := u.DB.Create(&user); result.Error != nil {
		log.Printf("error creating user: %s\n", result.Error)
		return result.Error
	}
	return nil
}

func (u *UserRepository) Update(user User) error {
	if result := u.DB.Save(&user); result.Error != nil {
		log.Printf("error updating user: %s\n", result.Error)
		return result.Error
	}
	return nil
}

func (u *UserRepository) FindByEmail(email string) (User, error) {
	return u.find("email", email)
}

func (u *UserRepository) FindByRecoveryUuid(recoveryUuid string) (User, error) {
	return u.find("recovery_uuid", recoveryUuid)
}

func (u *UserRepository) Admins() int64 {
	var totalRows int64
	u.DB.Where("role = ?", RoleAdmin).Take(&[]User{}).Count(&totalRows)
	return totalRows
}

func (u *UserRepository) Delete(uuid string) error {
	var user User
	result := u.DB.Where("uuid = ?", uuid).Delete(&user)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		log.Printf("error deleting user: %s\n", result.Error)
	}
	return nil
}

func Hash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return string(h.Sum(nil))
}

func (u *UserRepository) find(field, value string) (User, error) {
	var (
		err  error
		user User
	)
	result := u.DB.Limit(1).Where(fmt.Sprintf("%s = ?", field), value).Take(&user)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		err = result.Error
		log.Printf("error retrieving user: %s\n", result.Error)
	}
	return user, err
}
