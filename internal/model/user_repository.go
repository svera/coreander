package model

import (
	"crypto/sha256"
	"log"
	"time"

	"github.com/google/uuid"
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

func (u *UserRepository) Find(uuid string) (User, error) {
	user := User{}
	result := u.DB.Where("uuid = ?", uuid).Take(&user)
	if result.Error != nil {
		log.Printf("error retrieving user: %s\n", result.Error)
	}
	return user, result.Error
}

func (u *UserRepository) Create(user User) error {
	if result := u.DB.Create(&user); result.Error != nil {
		log.Printf("error creating user: %s\n", result.Error)
		return result.Error
	}
	return nil
}

func (u *UserRepository) Update(user User) error {
	user.Password = Hash(user.Password)
	if result := u.DB.Save(&user); result.Error != nil {
		log.Printf("error updating user: %s\n", result.Error)
		return result.Error
	}
	return nil
}

func (u *UserRepository) FindByEmail(email string) (User, error) {
	user := User{}
	result := u.DB.Where("email = ?", email).First(&user)
	if result.Error != nil {
		log.Printf("error retrieving user: %s\n", result.Error)
	}
	return user, result.Error
}

func (u *UserRepository) FindByRecoveryUuid(recoveryUuid string) (User, error) {
	user := User{}
	result := u.DB.Limit(1).Where("recovery_uuid = ?", recoveryUuid).First(&user)
	if result.Error != nil {
		log.Printf("error retrieving user by recovery ID: %s\n", result.Error)
	}
	return user, result.Error
}

func (u *UserRepository) Admins() int64 {
	var totalRows int64
	u.DB.Where("role = ?", RoleAdmin).Take(&[]User{}).Count(&totalRows)
	return totalRows
}

func (u *UserRepository) Delete(uuid string) error {
	user, err := u.Find(uuid)
	if err != nil {
		return nil
	}
	result := u.DB.Where("uuid = ?", uuid).Delete(&user)
	if result.Error != nil {
		log.Printf("error deleting user: %s\n", result.Error)
	}
	return nil
}

func (u *UserRepository) CheckCredentials(email, password string) (User, error) {
	var user User

	result := u.DB.Limit(1).Where("email = ? AND password = ?", email, Hash(password)).Find(&user)
	if result.Error != nil {
		log.Printf("error checking user credentials user: %s\n", result.Error)
	}
	return user, result.Error
}

func (u *UserRepository) GenerateRecovery(email string) (User, error) {
	recovery := User{
		RecoveryUUID:       uuid.NewString(),
		RecoveryValidUntil: time.Now().Add(24 * time.Hour),
	}

	result := u.DB.Limit(1).Where("email = ?", email).Updates(recovery)
	if result.Error != nil {
		log.Printf("error generating recovery: %s\n", result.Error)
	}

	if result.RowsAffected == 0 {
		return User{}, result.Error
	}

	return recovery, result.Error
}

func (u *UserRepository) ClearRecovery(email string) error {
	recovery := User{
		RecoveryUUID:       "",
		RecoveryValidUntil: time.Unix(0, 0),
	}

	result := u.DB.Limit(1).Where("email = ?", email).Updates(recovery)
	if result.Error != nil {
		log.Printf("error clearing recovery: %s\n", result.Error)
	}

	if result.RowsAffected == 0 {
		return result.Error
	}

	return result.Error
}

func Hash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return string(h.Sum(nil))
}
