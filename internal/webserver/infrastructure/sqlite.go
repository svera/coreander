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

	if err := db.AutoMigrate(&model.User{}, &model.Highlight{}, &model.History{}); err != nil {
		log.Fatal(err)
	}
	addDefaultAdmin(db, wordsPerMinute)
	if err := MigrateReadingToHistory(db); err != nil {
		log.Fatal(err)
	}
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

// MigrateReadingToHistory migrates all data from the reading table to the history table
// with action set to HistoryActionRead. This is a one-time migration function.
func MigrateReadingToHistory(db *gorm.DB) error {
	// Check if reading table exists
	if !db.Migrator().HasTable("readings") {
		log.Println("Reading table does not exist, skipping migration")
		return nil
	}

	// Check if history table exists
	if !db.Migrator().HasTable("history") {
		return fmt.Errorf("history table does not exist")
	}

	// Get all reading records
	var readings []model.Reading
	if err := db.Table("readings").Find(&readings).Error; err != nil {
		return fmt.Errorf("failed to fetch reading records: %w", err)
	}

	if len(readings) == 0 {
		log.Println("No reading records to migrate")
		return nil
	}

	// Migrate each reading record to history
	for _, reading := range readings {
		history := model.History{
			UserID: reading.UserID,
			Path:   reading.Path,
			Action: model.HistoryActionRead,
		}
		// Use raw SQL to preserve timestamps
		if err := db.Exec(
			"INSERT INTO history (user_id, path, action, created_at, updated_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(user_id, path, action) DO UPDATE SET updated_at = excluded.updated_at",
			history.UserID,
			history.Path,
			history.Action,
			reading.CreatedAt,
			reading.UpdatedAt,
		).Error; err != nil {
			return fmt.Errorf("failed to migrate reading record (user_id: %d, path: %s): %w", reading.UserID, reading.Path, err)
		}
	}

	log.Printf("Successfully migrated %d reading records to history table", len(readings))
	return nil
}
