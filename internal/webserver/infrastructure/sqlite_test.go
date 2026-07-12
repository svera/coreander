package infrastructure_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
	"github.com/svera/coreander/v5/internal/webserver/model"
	"gorm.io/gorm"
)

// legacyUser and legacyInvitation mirror model.User/model.Invitation as they were before email columns
// were declared case-insensitive (see model.User.Email, model.Invitation.Email). Migrating a database
// through these shapes first, then through infrastructure.Connect, simulates a real database created by
// an older version of the app being opened by the current one.
type legacyUser struct {
	ID                 uint `gorm:"primarykey"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Uuid               string `gorm:"uniqueIndex; not null"`
	Name               string `gorm:"not null"`
	Username           string `gorm:"type:text collate nocase; not null; unique"`
	Email              string `gorm:"uniqueIndex; not null"`
	SendToEmail        string
	Password           string
	Role               int `gorm:"not null"`
	WordsPerMinute     float64
	RecoveryUUID       string
	RecoveryValidUntil time.Time
	LastRequest        time.Time
	ShowFileName       bool   `gorm:"default:false; not null"`
	PrivateProfile     int    `gorm:"default:0; not null"`
	PreferredEpubType  string `gorm:"default:'epub'; not null"`
	DefaultAction      string `gorm:"default:'download'; not null"`
	Language           string
}

func (legacyUser) TableName() string { return "users" }

type legacyInvitation struct {
	ID         uint   `gorm:"primarykey"`
	Email      string `gorm:"uniqueIndex; not null"`
	UUID       string `gorm:"uniqueIndex; not null"`
	ValidUntil time.Time
}

func (legacyInvitation) TableName() string { return "invitations" }

// seedLegacySchema creates users/invitations tables shaped like they were before email columns were made
// case-insensitive, with an email stored in mixed case in each, plus a highlight and a reading that hold
// real foreign key references into users - so the users table rebuild triggered by the collation fix has
// something real to potentially break.
func seedLegacySchema(t *testing.T, path string) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	defer sqlDB.Close()

	if err := db.AutoMigrate(&legacyUser{}, &model.Highlight{}, &model.Reading{}, &legacyInvitation{}); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}

	user := legacyUser{
		Uuid:     "u1",
		Name:     "Existing",
		Username: "existing",
		Email:    "Mixed@Case.com",
		Password: "x",
		Role:     model.RoleRegular,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed legacy user: %v", err)
	}

	if err := db.Create(&model.Highlight{UserID: int(user.ID), Slug: "some-book"}).Error; err != nil {
		t.Fatalf("seed highlight referencing user: %v", err)
	}
	if err := db.Create(&model.Reading{UserID: int(user.ID), Slug: "some-book"}).Error; err != nil {
		t.Fatalf("seed reading referencing user: %v", err)
	}

	if err := db.Create(&legacyInvitation{
		Email:      "Invited@Example.COM",
		UUID:       "inv1",
		ValidUntil: time.Now().Add(24 * time.Hour),
	}).Error; err != nil {
		t.Fatalf("seed legacy invitation: %v", err)
	}
}

func TestConnect_NormalizesExistingEmailCase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	seedLegacySchema(t, path)

	db := infrastructure.Connect(path, 250)

	var storedEmail string
	if err := db.Raw("SELECT email FROM users WHERE uuid = ?", "u1").Scan(&storedEmail).Error; err != nil {
		t.Fatalf("query stored email: %v", err)
	}
	if storedEmail != "mixed@case.com" {
		t.Errorf("expected existing email to be lowercased to %q, got %q", "mixed@case.com", storedEmail)
	}

	var invEmail string
	if err := db.Raw("SELECT email FROM invitations WHERE uuid = ?", "inv1").Scan(&invEmail).Error; err != nil {
		t.Fatalf("query stored invitation email: %v", err)
	}
	if invEmail != "invited@example.com" {
		t.Errorf("expected existing invitation email to be lowercased to %q, got %q", "invited@example.com", invEmail)
	}

	repo := &model.UserRepository{DB: db}
	user, err := repo.FindByEmail("MIXED@CASE.COM")
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	if user == nil || user.Uuid != "u1" {
		t.Errorf("expected to find existing user u1 by a differently-cased email, got %+v", user)
	}

	// The rebuild must preserve the table's other constraints (gorm bakes them in at CREATE TABLE time; a
	// naive rebuild under a temporary table name would instead produce differently-named ones).
	var usersSchema string
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'users'").Scan(&usersSchema).Error; err != nil {
		t.Fatalf("query users schema: %v", err)
	}
	for _, want := range []string{"`email` text collate nocase", "CONSTRAINT `uni_users_username`"} {
		if !strings.Contains(usersSchema, want) {
			t.Errorf("expected users schema to contain %q, got %q", want, usersSchema)
		}
	}

	// Standalone indexes (email/uuid uniqueness included) must survive the rebuild too.
	var indexCount int64
	if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND tbl_name = 'users' AND name IN ('idx_users_email','idx_users_uuid')").Scan(&indexCount).Error; err != nil {
		t.Fatalf("query users indexes: %v", err)
	}
	if indexCount != 2 {
		t.Errorf("expected both original indexes to be restored on users, found %d", indexCount)
	}

	// highlights/readings hold real foreign key references into users; the rebuild must not have broken them.
	rows, err := db.Raw("PRAGMA foreign_key_check").Rows()
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Error("expected no foreign key violations after rebuild")
	}

	// Uniqueness must actually be enforced case-insensitively now, not just declared.
	if err := db.Create(&legacyUser{
		Uuid: "u2", Name: "Dup", Username: "dup", Email: "mixed@CASE.com", Password: "x", Role: model.RoleRegular,
	}).Error; err == nil {
		t.Error("expected inserting a case-variant duplicate email to be rejected")
	}
}

func TestConnect_SkipsNormalizationOnExistingCaseCollision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collision.db")
	seedLegacySchema(t, path)

	// Add a second row whose email only differs in case from the first, directly at the DB level -
	// simulating data that could exist precisely because of the bug being fixed here.
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("reopen legacy db: %v", err)
	}
	if err := db.Create(&legacyUser{
		Uuid:     "u2",
		Name:     "Existing2",
		Username: "existing2",
		Email:    "mixed@CASE.com",
		Password: "x",
		Role:     model.RoleRegular,
	}).Error; err != nil {
		t.Fatalf("seed colliding row: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.Close()

	gotDB := infrastructure.Connect(path, 250)

	var emails []string
	if err := gotDB.Raw("SELECT email FROM users WHERE uuid IN ('u1','u2') ORDER BY uuid").Scan(&emails).Error; err != nil {
		t.Fatalf("query stored emails: %v", err)
	}
	if len(emails) != 2 || emails[0] != "Mixed@Case.com" || emails[1] != "mixed@CASE.com" {
		t.Errorf("expected colliding emails to be left untouched, got %v", emails)
	}
}
