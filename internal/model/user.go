package model

import (
	"fmt"
	"net/mail"

	"gorm.io/gorm"
)

const (
	RoleRegular = iota + 1
	RoleAdmin
)

type User struct {
	gorm.Model
	Uuid           string `gorm:"uniqueIndex"`
	Name           string
	Email          string `gorm:"uniqueIndex"`
	SendToEmail    string
	Password       string
	Role           int
	WordsPerMinute float64
}

// Validate checks all user's fields to ensure they are in the required format
func (u User) Validate(minPasswordLength int) map[string]string {
	errs := map[string]string{}

	if u.Name == "" {
		errs["name"] = "Name cannot be empty"
	}

	if u.WordsPerMinute < 1 || u.WordsPerMinute > 999 {
		errs["wordsperminute"] = "Incorrect reading speed"
	}

	if _, err := mail.ParseAddress(u.Email); err != nil {
		errs["email"] = "Incorrect email address"
	}

	if _, err := mail.ParseAddress(u.SendToEmail); u.SendToEmail != "" && err != nil {
		errs["sendtoemail"] = "Incorrect send to email address"
	}

	if u.Role < RoleRegular || u.Role > RoleAdmin {
		errs["role"] = "Incorrect role"
	}

	if len(u.Password) < minPasswordLength {
		errs["password"] = fmt.Sprintf("Password must be longer than %d characters", minPasswordLength)
	}

	return errs
}
