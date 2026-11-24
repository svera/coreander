package model

import (
	"time"

	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v5/internal/precisiondate"
)

// AuthorCandidates holds search results and the latest possible publication date
// of the author's earliest dated document. A zero date means no constraint.
type AuthorCandidates struct {
	IDs                            []string
	FirstPublicationDateUpperBound date.Date
}

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
