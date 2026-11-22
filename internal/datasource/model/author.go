package model

import (
	"time"

	"github.com/svera/coreander/v5/internal/precisiondate"
)

// Gender values are persisted in the author index; keep their numeric values stable.
const (
	GenderUnknown = iota
	GenderMale
	GenderFemale
	GenderIntersex
	GenderTransgenderFemale
	GenderTransgenderMale
)

type Author interface {
	BirthName() string
	Description(language string) string
	InstanceOf() float64
	Gender() float64
	DateOfBirth() precisiondate.PrecisionDate
	DateOfDeath() precisiondate.PrecisionDate
	Image() string
	// Links returns source-owned profile IDs and URLs, including language-specific keys.
	Links() map[string]string
	// OwnsLink identifies entries replaced by a metadata refresh, even if absent from Links.
	OwnsLink(key string) bool
	RetrievedOn() time.Time
	Pseudonyms() []string
}
