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

	// AutoMigrate can decide a table needs a full rebuild (e.g. to add a missing/renamed constraint), which
	// briefly drops and recreates it. If another table holds an enforced foreign key into it (highlights/
	// readings -> users here) and has rows actually referencing it, SQLite correctly refuses that DROP with
	// foreign keys on. gorm's own AlterColumn already guards itself against this (see fixEmailCollation
	// below); AutoMigrate's own constraint-adding path does not, so it's wrapped the same way here.
	migrateErr := runWithoutForeignKeys(db, func() error {
		return db.AutoMigrate(&model.User{}, &model.Highlight{}, &model.Reading{}, &model.Invitation{})
	})
	if migrateErr != nil {
		log.Fatal(migrateErr)
	}
	fixEmailCollation(db, "users", &model.User{})
	fixEmailCollation(db, "invitations", &model.Invitation{})
	applySQLiteIndexes(db)
	addDefaultAdmin(db, wordsPerMinute)
	return db
}

// fixEmailCollation brings table's email column in line with today's schema (declared "collate nocase", see
// model.User/model.Invitation) for databases created before that change, since SQLite can't alter a column's
// collation via a plain ALTER TABLE, and gorm's own AutoMigrate doesn't revisit a column's type/collation when
// it only needs to add a missing constraint elsewhere.
//
// Uses gorm's own Migrator.AlterColumn, which rebuilds the table internally (rename/copy/drop, with foreign
// keys disabled around it) while preserving the table's existing constraint names, e.g. fk_highlights_shared_by
// - unlike a hand-rolled rebuild under a temporary table name, which bakes in new, table-specific names
// instead. AlterColumn's rebuild also drops the table's standalone indexes as a side effect (it only
// reconstructs what is declared directly on the CREATE TABLE statement), so a follow-up AutoMigrate restores
// them, including the ones enforcing email/UUID uniqueness.
//
// Skipped (with a warning) if two existing rows' emails already only differ by case, since the restored
// unique index would then reject one of them and this won't guess which one to keep.
func fixEmailCollation(db *gorm.DB, table string, value any) {
	var schemaSQL string
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&schemaSQL).Error; err != nil {
		log.Printf("warning: could not read %s schema: %v\n", table, err)
		return
	}
	if schemaSQL == "" {
		return // table doesn't exist yet; AutoMigrate already created it with today's (correct) schema.
	}
	if strings.Contains(schemaSQL, "`email` text collate nocase") {
		return // already migrated.
	}

	var collisions int64
	if err := db.Raw(fmt.Sprintf(`
		SELECT COUNT(*) FROM (
			SELECT 1 FROM %s GROUP BY LOWER(email) HAVING COUNT(*) > 1
		)
	`, table)).Scan(&collisions).Error; err != nil {
		log.Printf("warning: could not check %s for case-duplicate emails: %v\n", table, err)
		return
	}
	if collisions > 0 {
		log.Printf("warning: %s has rows whose email only differs by case; leaving email case-sensitive for this table until resolved manually\n", table)
		return
	}

	// Lowercase data before fixing the column's collation below: once it's "collate nocase", a plain
	// "email != LOWER(email)" always evaluates false (both sides compare equal under nocase), silently
	// turning this into a no-op.
	if err := db.Exec(fmt.Sprintf("UPDATE %s SET email = LOWER(email) WHERE email != LOWER(email)", table)).Error; err != nil {
		log.Printf("warning: could not lowercase existing emails in %s: %v\n", table, err)
		return
	}
	if err := db.Migrator().AlterColumn(value, "Email"); err != nil {
		log.Printf("warning: could not fix %s.email collation: %v\n", table, err)
		return
	}
	if err := db.AutoMigrate(value); err != nil {
		log.Printf("warning: could not restore indexes on %s after collation fix: %v\n", table, err)
	}
}

// runWithoutForeignKeys disables foreign key enforcement for the duration of fc, restoring it afterwards -
// same technique gorm's own Migrator.AlterColumn uses internally (see fixEmailCollation above), needed here
// because Migrator.AutoMigrate's own constraint-adding path doesn't guard itself the same way, and a table
// rebuild it triggers can otherwise be rejected by a real foreign key reference from another table.
func runWithoutForeignKeys(db *gorm.DB, fc func() error) error {
	var enabled int
	db.Raw("PRAGMA foreign_keys").Scan(&enabled)
	if enabled == 1 {
		db.Exec("PRAGMA foreign_keys = OFF")
		defer db.Exec("PRAGMA foreign_keys = ON")
	}
	return fc()
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
