package model

import (
	"fmt"
	"net/mail"

	"gorm.io/gorm"
)

const (
	RoleRegular = 1
	RoleAdmin   = 2
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
func (u User) Validate(minPasswordLength int) []string {
	errs := []string{}

	if u.Name == "" {
		errs = append(errs, "Name cannot be empty")
	}

	if u.WordsPerMinute < 1 || u.WordsPerMinute > 999 {
		errs = append(errs, "Incorrect reading speed")
	}

	if _, err := mail.ParseAddress(u.Email); err != nil {
		errs = append(errs, "Incorrect email address")
	}

	if _, err := mail.ParseAddress(u.SendToEmail); u.SendToEmail != "" && err != nil {
		errs = append(errs, "Incorrect send to email address")
	}

	if u.Role < 1 || u.Role > 2 {
		errs = append(errs, "Incorrect role")
	}

	if len(u.Password) < minPasswordLength {
		errs = append(errs, fmt.Sprintf("Password must be longer than %d characters", minPasswordLength))
	}

	return errs
}
