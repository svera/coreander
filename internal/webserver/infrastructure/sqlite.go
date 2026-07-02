package infrastructure

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/svera/coreander/v5/internal/webserver/model"
	"gorm.io/gorm"
)

func Connect(path string, wordsPerMinute float64) *gorm.DB {
	if _, err := os.Stat(path); os.IsNotExist(err) && !strings.Contains(path, ":memory:") {
		if _, err = os.Create(path); err != nil {
			log.Fatal(err)
		}
		log.Printf("Created database at %s\n", path)
	}

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=cache_size(-64000)", path)), &gorm.Config{
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := db.AutoMigrate(&model.User{}, &model.Highlight{}, &model.Reading{}, &model.Invitation{}); err != nil {
		log.Fatal(err)
	}
	applySQLiteIndexes(db)
	addDefaultAdmin(db, wordsPerMinute)
	return db
}

func applySQLiteIndexes(db *gorm.DB) {
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_readings_user_in_progress_partial ON readings(user_id, updated_at) WHERE completed_on IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_readings_user_completed_partial ON readings(user_id, completed_on) WHERE completed_on IS NOT NULL`,
	}
	for _, stmt := range indexes {
		if err := db.Exec(stmt).Error; err != nil {
			log.Printf("warning: creating sqlite index: %v", err)
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
