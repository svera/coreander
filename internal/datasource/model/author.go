package model

import (
	"time"

	"github.com/svera/coreander/v4/internal/precisiondate"
)

type Author interface {
	BirthName() string
	Description(language string) string
	InstanceOf() float64
	Gender() float64
	DateOfBirth() precisiondate.PrecisionDate
	DateOfDeath() precisiondate.PrecisionDate
	Image() string
	Website() string
	WikipediaLink(language string) string
	SourceID() string
	RetrievedOn() time.Time
	Pseudonyms() []string
}
