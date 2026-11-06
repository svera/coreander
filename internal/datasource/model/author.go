package model

import (
	"time"

	"github.com/svera/coreander/v5/internal/precisiondate"
)

type Author interface {
	BirthName() string
	Description(language string) string
	InstanceOf() float64
	Gender() float64
	DateOfBirth() precisiondate.PrecisionDate
	DateOfDeath() precisiondate.PrecisionDate
	Image() string
	// Links returns the current links owned by this datasource, including language-specific keys.
	Links() map[string]string
	// OwnsLink identifies entries replaced by a metadata refresh, even if absent from Links.
	OwnsLink(key string) bool
	SourceID() string
	RetrievedOn() time.Time
	Pseudonyms() []string
}
