package model

import (
	"crypto/sha256"

	"gorm.io/gorm"
)

type Auth struct {
	DB *gorm.DB
}

func (a *Auth) CheckCredentials(username, password string) (User, error) {
	var user User

	result := a.DB.Where("username = ? AND password = ?", username, Hash(password)).Take(&user)
	return user, result.Error
}

func Hash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return string(h.Sum(nil))
}
