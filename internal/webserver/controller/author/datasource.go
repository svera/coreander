package author

import (
	"time"

	"github.com/rickb777/date/v2"
)

type Author interface {
	Name(language string) string
	BirthName() string
	Description(language string) string
	InstanceOf() int
	Gender() int
	DateOfBirth() date.Date
	YearOfBirth() int
	DateOfDeath() date.Date
	YearOfDeath() int
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
