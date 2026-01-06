package infrastructure

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"gorm.io/gorm"
)

func Connect(path string, wordsPerMinute float64) *gorm.DB {
	if _, err := os.Stat(path); os.IsNotExist(err) && !strings.Contains(path, ":memory:") {
		if _, err = os.Create(path); err != nil {
			log.Fatal(err)
		}
		log.Printf("Created database at %s\n", path)
	}

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("%s?_pragma=foreign_keys(1)", path)), &gorm.Config{
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Run column rename migration before AutoMigrate to avoid creating duplicate columns
	migrateLastLoginToLastRequest(db)

	if err := db.AutoMigrate(&model.User{}, &model.Highlight{}, &model.Reading{}, &model.Invitation{}); err != nil {
		log.Fatal(err)
	}
	addDefaultAdmin(db, wordsPerMinute)
	return db
}

func migrateLastLoginToLastRequest(db *gorm.DB) {
	migrator := db.Migrator()

	// Check if the old column exists in the database using SQLite pragma
	var count int
	err := db.Raw("SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'last_login'").Scan(&count).Error
	if err != nil {
		// If we can't check, skip migration (might be a new database)
		return
	}

	if count > 0 {
		log.Println("Migrating last_login column to last_request...")
		// RenameColumn uses database column names (snake_case)
		if err := migrator.RenameColumn(&model.User{}, "last_login", "last_request"); err != nil {
			log.Printf("Error renaming column last_login to last_request: %v\n", err)
			// Don't fatal here, just log the error - the column might already be renamed
		} else {
			log.Println("Successfully renamed last_login to last_request")
		}
	}
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
