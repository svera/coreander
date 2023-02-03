package infrastructure

import (
	"log"
	"os"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/svera/coreander/internal/model"
	"gorm.io/gorm"
)

func Connect(path string) *gorm.DB {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if _, err = os.Create(path); err != nil {
			log.Fatal(err)
		}
		log.Printf("Created database at %s\n", path)
	}

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	db.AutoMigrate(&model.User{})
	addDefaultAdmin(db)
	return db
}

func addDefaultAdmin(db *gorm.DB) {
	var result int64
	db.Table("users").Count(&result)

	if result == 0 {
		user := &model.User{
			Uuid:     uuid.NewString(),
			Name:     "Admin",
			Username: "admin",
			Password: model.Hash("admin"),
			Role:     model.RoleAdmin,
		}
		result := db.Create(&user)
		if result.Error != nil {
			log.Fatal("Couldn't create default admin")
		}
	}
}
