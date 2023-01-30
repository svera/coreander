package model

import (
	"crypto/sha256"

	"gorm.io/gorm"
)

const (
	RoleRegular = 0
	RoleAdmin   = 1
)

type User struct {
	gorm.Model
	Name     string
	Username string
	Password string
	Role     int
}

type Auth struct {
	DB *gorm.DB
}

func (a *Auth) CheckCredentials(username, password string) bool {
	var user User

	result := a.DB.Where("username = ? AND password = ?", username, Hash(password)).Take(&user)
	return result.RowsAffected == 1
}

func Hash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return string(h.Sum(nil))
}
