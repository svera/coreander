package model

import (
	"crypto/sha256"

	"gorm.io/gorm"
)

type Auth struct {
	DB *gorm.DB
}

type UserData struct {
	Name  string
	Email string
	Uuid  string
	Role  int
}

func (a *Auth) CheckCredentials(email, password string) (User, error) {
	var user User

	result := a.DB.Where("email = ? AND password = ?", email, Hash(password)).Take(&user)
	return user, result.Error
}

func Hash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return string(h.Sum(nil))
}
