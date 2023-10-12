package model

import (
	"log"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HighlightRepository struct {
	DB *gorm.DB
}

func (u *HighlightRepository) Highlights(userID int, page int, resultsPerPage int) ([]Highlight, error) {
	highlights := []Highlight{}
	result := u.DB.Scopes(Paginate(page, resultsPerPage)).Select("path").Where("user_id = ?", userID).Order("created_at DESC").Find(&highlights)
	if result.Error != nil {
		log.Printf("error listing highlights: %s\n", result.Error)
	}
	return highlights, result.Error
}

func (u *HighlightRepository) Highlighted(userID int, documentPath string) bool {
	var count int64
	u.DB.Select("path").Where("user_id = ? and PATH = ?", userID, documentPath).Count(&count)
	return count == 1
}

func (u *HighlightRepository) Highlight(userID int, documentPath string) error {
	highlight := Highlight{
		UserID: userID,
		Path:   documentPath,
	}
	return u.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&highlight).Error
}

func (u *HighlightRepository) Remove(userID int, documentPath string) error {
	highlight := Highlight{
		UserID: userID,
		Path:   documentPath,
	}
	return u.DB.Delete(&highlight).Error
}

/*
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
	result := u.DB.Limit(1).Where(fmt.Sprintf("%s = ?", field), value).Find(&user)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		err = result.Error
		log.Printf("error retrieving user: %s\n", result.Error)
	}
	return user, err
}
*/
