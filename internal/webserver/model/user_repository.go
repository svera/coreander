package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/svera/coreander/v4/internal/result"
	"gorm.io/gorm"
)

type UserRepository struct {
	DB *gorm.DB
}

func (u *UserRepository) List(page int, resultsPerPage int) (result.Paginated[[]User], error) {
	var users []User

	res := u.DB.Scopes(Paginate(page, resultsPerPage)).Order("email ASC").Find(&users)
	if res.Error != nil {
		log.Printf("error listing users: %s\n", res.Error)
		return result.Paginated[[]User]{}, res.Error
	}

	return result.NewPaginated(
		resultsPerPage,
		page,
		int(u.Total()),
		users,
	), nil
}

func (u *UserRepository) Total() int64 {
	var (
		totalRows int64
		users     []User
	)

	u.DB.Model(&users).Count(&totalRows)
	return totalRows
}

func (u *UserRepository) FindByUuid(uuid string) (*User, error) {
	return u.find("uuid", uuid)
}

func (u *UserRepository) Create(user *User) error {
	if result := u.DB.Create(user); result.Error != nil {
		log.Printf("error creating user: %s\n", result.Error)
		return result.Error
	}
	return nil
}

func (u *UserRepository) Update(user *User) error {
	if result := u.DB.Save(user); result.Error != nil {
		log.Printf("error updating user: %s\n", result.Error)
		return result.Error
	}
	return nil
}

func (u *UserRepository) UpdateLastRequest(userID uint) error {
	if userID == 0 {
		return nil
	}
	var user User
	if err := u.DB.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	user.LastRequest = time.Now().UTC()
	return u.Update(&user)
}

func (u *UserRepository) FindByEmail(email string) (*User, error) {
	return u.find("email", email)
}

func (u *UserRepository) FindByUsername(username string) (*User, error) {
	return u.find("username", username)
}

func (u *UserRepository) FindByRecoveryUuid(recoveryUuid string) (*User, error) {
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

func (u *UserRepository) find(field, value string) (*User, error) {
	var user User

	result := u.DB.Where(fmt.Sprintf("%s = ?", field), value).First(&user)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, result.Error
}
