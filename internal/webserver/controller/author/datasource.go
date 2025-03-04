package author

import (
	"time"

	"github.com/svera/coreander/v4/internal/precisiondate"
)

type Author interface {
	BirthName() string
	Description(language string) string
	InstanceOf() int
	Gender() int
	DateOfBirth() precisiondate.PrecisionDate
	DateOfDeath() precisiondate.PrecisionDate
	Image() string
	Website() string
	WikipediaLink(language string) string
	SourceID() string
	RetrievedOn() time.Time
	Pseudonyms() []string
}

type DataSource interface {
	SearchAuthor(name string, languages []string) (Author, error)
	RetrieveAuthor(IDs []string, languages []string) (Author, error)
}
