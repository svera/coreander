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

// resolveSlug maps a legacy document path (file ID) to its slug; required when upgrading an older
// database that stored paths. Pass nil for tests or fresh databases (no path-based tables).
func Connect(path string, wordsPerMinute float64, resolveSlug func(string) string) *gorm.DB {
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

	// Legacy DBs stored document path instead of slug. SQLite cannot ADD COLUMN ... NOT NULL on
	// non-empty tables without a default. Add nullable slug, backfill, rebuild PK, then AutoMigrate.
	prepareLegacySlugColumns(db)
	if resolveSlug != nil {
		FillSlugsFromPaths(db, resolveSlug)
		MigrateHighlightsToSlugPK(db)
		MigrateReadingsToSlugPK(db)
	}

	if err := db.AutoMigrate(&model.User{}, &model.Highlight{}, &model.Reading{}, &model.Invitation{}); err != nil {
		log.Fatal(err)
	}
	addDefaultAdmin(db, wordsPerMinute)
	return db
}

// prepareLegacySlugColumns adds a nullable slug column when the table still uses path,
// so AutoMigrate never issues ADD slug NOT NULL (which SQLite rejects for existing rows).
func prepareLegacySlugColumns(db *gorm.DB) {
	addNullableSlugIfPathExists := func(table string) {
		var tableExists int
		if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&tableExists).Error; err != nil || tableExists == 0 {
			return
		}
		var pathCol int
		if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'path'", table).Scan(&pathCol).Error; err != nil || pathCol == 0 {
			return
		}
		var slugCol int
		if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'slug'", table).Scan(&slugCol).Error; err != nil || slugCol > 0 {
			return
		}
		if err := db.Exec("ALTER TABLE `" + table + "` ADD COLUMN slug text").Error; err != nil {
			log.Printf("prepareLegacySlugColumns %s: %v\n", table, err)
		}
	}
	addNullableSlugIfPathExists("highlights")
	addNullableSlugIfPathExists("readings")
}

// FillSlugsFromPaths updates empty slug fields in highlights and readings tables
// by resolving each path (document ID) to its slug via the given resolver.
// resolveSlug should return the document slug for a given path, or empty string if not found.
// For highlights, only runs if the table still has a path column (pre-migration).
func FillSlugsFromPaths(db *gorm.DB, resolveSlug func(path string) string) {
	fillHighlightsSlugs(db, resolveSlug)
	fillReadingsSlugs(db, resolveSlug)
}

func fillHighlightsSlugs(db *gorm.DB, resolveSlug func(path string) string) {
	var pathColExists int
	if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info('highlights') WHERE name = 'path'").Scan(&pathColExists).Error; err != nil || pathColExists == 0 {
		return
	}
	var rows []struct {
		UserID int
		Path   string
	}
	if err := db.Raw("SELECT user_id, path FROM highlights WHERE slug = ? OR slug IS NULL", "").Scan(&rows).Error; err != nil {
		return
	}
	for _, r := range rows {
		if slug := resolveSlug(r.Path); slug != "" {
			db.Exec("UPDATE highlights SET slug = ? WHERE user_id = ? AND path = ?", slug, r.UserID, r.Path)
		}
	}
}

// MigrateHighlightsToSlugPK migrates the highlights table from (user_id, path) to (user_id, slug) primary key.
// Run after FillSlugsFromPaths so slugs are populated. No-op if the table has already been migrated.
func MigrateHighlightsToSlugPK(db *gorm.DB) {
	var pathColExists int
	if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info('highlights') WHERE name = 'path'").Scan(&pathColExists).Error; err != nil || pathColExists == 0 {
		return
	}
	log.Println("Migrating highlights table to slug primary key...")
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			CREATE TABLE highlights_new (
				user_id INTEGER NOT NULL,
				slug TEXT NOT NULL,
				created_at DATETIME,
				updated_at DATETIME,
				shared_by_id INTEGER,
				comment TEXT,
				PRIMARY KEY (user_id, slug)
			)
		`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO highlights_new (user_id, slug, created_at, updated_at, shared_by_id, comment)
			SELECT user_id, slug, created_at, updated_at, shared_by_id, comment
			FROM highlights
			WHERE slug IS NOT NULL AND slug != ''
		`).Error; err != nil {
			return err
		}
		if err := tx.Exec("DROP TABLE highlights").Error; err != nil {
			return err
		}
		if err := tx.Exec("ALTER TABLE highlights_new RENAME TO highlights").Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		log.Printf("Error migrating highlights table: %v\n", err)
		return
	}
	log.Println("Successfully migrated highlights table to slug primary key")
}

func fillReadingsSlugs(db *gorm.DB, resolveSlug func(path string) string) {
	var pathColExists int
	if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info('readings') WHERE name = 'path'").Scan(&pathColExists).Error; err != nil || pathColExists == 0 {
		return
	}
	var rows []struct {
		UserID int
		Path   string
	}
	if err := db.Raw("SELECT user_id, path FROM readings WHERE slug = ? OR slug IS NULL", "").Scan(&rows).Error; err != nil {
		return
	}
	for _, r := range rows {
		if slug := resolveSlug(r.Path); slug != "" {
			db.Exec("UPDATE readings SET slug = ? WHERE user_id = ? AND path = ?", slug, r.UserID, r.Path)
		}
	}
}

// MigrateReadingsToSlugPK migrates the readings table from (user_id, path) to (user_id, slug) primary key.
// Run after FillSlugsFromPaths so slugs are populated. No-op if the table has already been migrated.
func MigrateReadingsToSlugPK(db *gorm.DB) {
	var pathColExists int
	if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info('readings') WHERE name = 'path'").Scan(&pathColExists).Error; err != nil || pathColExists == 0 {
		return
	}
	log.Println("Migrating readings table to slug primary key...")
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			CREATE TABLE readings_new (
				user_id INTEGER NOT NULL,
				slug TEXT NOT NULL,
				created_at DATETIME,
				updated_at DATETIME,
				position TEXT,
				completed_on DATETIME,
				PRIMARY KEY (user_id, slug)
			)
		`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO readings_new (user_id, slug, created_at, updated_at, position, completed_on)
			SELECT user_id, slug, created_at, updated_at, position, completed_on
			FROM readings
			WHERE slug IS NOT NULL AND slug != ''
		`).Error; err != nil {
			return err
		}
		if err := tx.Exec("DROP TABLE readings").Error; err != nil {
			return err
		}
		if err := tx.Exec("ALTER TABLE readings_new RENAME TO readings").Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		log.Printf("Error migrating readings table: %v\n", err)
		return
	}
	log.Println("Successfully migrated readings table to slug primary key")
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
