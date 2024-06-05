package infrastructure

import (
	"log"
	"os"
	"strings"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"gorm.io/gorm"
)

func Connect(path string, wordsPerMinute float64) *gorm.DB {
	if _, err := os.Stat(path); os.IsNotExist(err) && !strings.Contains(path, "file::memory") {
		if _, err = os.Create(path); err != nil {
			log.Fatal(err)
		}
		log.Printf("Created database at %s\n", path)
	}

	// Use the following line to connect when the temporary code block below is removed
	//db, err := gorm.Open(sqlite.Open(fmt.Sprintf("%s?_pragma=foreign_keys(1)", path)), &gorm.Config{})
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	if res := db.Exec("PRAGMA foreign_keys(1)", nil); res.Error != nil {
		log.Fatal(err)
	}

	if err := db.AutoMigrate(&model.User{}, &model.Highlight{}); err != nil {
		log.Fatal(err)
	}
	addDefaultAdmin(db, wordsPerMinute)
	return db
}

func addDefaultAdmin(db *gorm.DB, wordsPerMinute float64) {
	var result int64
	db.Table("users").Count(&result)

	if result == 0 {
		user := &model.User{
			Uuid:           uuid.NewString(),
			Name:           "Admin",
			Username:       "admin",
			Email:          "admin@example.com",
			Password:       model.Hash("admin"),
			Role:           model.RoleAdmin,
			WordsPerMinute: wordsPerMinute,
		}
		result := db.Create(&user)
		if result.Error != nil {
			log.Fatal("Couldn't create default admin")
		}
	}
}
