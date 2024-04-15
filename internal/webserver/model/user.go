package model

import (
	"net/mail"
	"regexp"
	"time"
)

const (
	RoleRegular = iota + 1
	RoleAdmin
)

const UsernamePattern = `^[A-z0-9_\-.]+$`

type User struct {
	ID                 uint `gorm:"primarykey"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Uuid               string `gorm:"uniqueIndex"`
	Name               string
	Username           string `gorm:"type:text collate nocase; not null; default:''; unique"`
	Email              string `gorm:"uniqueIndex"`
	SendToEmail        string
	Password           string
	Role               int
	WordsPerMinute     float64
	RecoveryUUID       string
	RecoveryValidUntil time.Time
	Highlights         []Highlight `gorm:"constraint:OnDelete:CASCADE"`
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

	if u.Username == "" {
		errs["username"] = "Username cannot be empty"
	}

	if len(u.Username) > 20 {
		errs["username"] = "Username cannot be longer than 20 characters"
	}

	if match, _ := regexp.Match(UsernamePattern, []byte(u.Username)); u.Username != "" && !match {
		errs["username"] = "Username can only have letters, numbers, _, - and ."
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
