package model

import (
	"net/mail"
	"time"

	"gorm.io/gorm"
)

const (
	RoleRegular = iota + 1
	RoleAdmin
)

type User struct {
	gorm.Model
	Uuid               string `gorm:"uniqueIndex"`
	Name               string
	Email              string `gorm:"uniqueIndex"`
	SendToEmail        string
	Password           string
	Role               int
	WordsPerMinute     float64
	RecoveryUUID       string
	RecoveryValidUntil time.Time
}

// Validate checks all user's fields to ensure they are in the required format
func (u User) Validate(minPasswordLength int) map[string]string {
	errs := map[string]string{}

	if u.Name == "" {
		errs["name"] = "Name cannot be empty"
	}

	if len(u.Name) > 50 {
		errs["name"] = "Name cannot be longer than 50 characters"
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

	if len(u.Email) > 100 {
		errs["email"] = "Email cannot be longer than 100 characters"
	}

	if len(u.SendToEmail) > 100 {
		errs["sendtoemail"] = "Send to email cannot be longer than 100 characters"
	}

	if u.Role < RoleRegular || u.Role > RoleAdmin {
		errs["role"] = "Incorrect role"
	}

	if len(u.Password) < minPasswordLength {
		errs["password"] = "Password must be longer than %d characters"
	}

	if len(u.Password) > 50 {
		errs["password"] = "Password cannot be longer than 50 characters"
	}

	return errs
}

func (u User) ConfirmPassword(confirmPassword string, minPasswordLength int, errs map[string]string) map[string]string {
	if len(u.Password) < minPasswordLength {
		errs["password"] = "Password must be longer than %d characters"
	}

	if confirmPassword == "" {
		errs["confirmpassword"] = "Confirm password cannot be empty"
	}

	if u.Password != confirmPassword {
		errs["confirmpassword"] = "Password and confirmation do not match"
	}

	return errs
}
