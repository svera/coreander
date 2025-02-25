package author

import (
	"time"

	"github.com/rickb777/date/v2"
)

type Author interface {
	Description(language string) string
	InstanceOf() int
	Gender() int
	DateOfBirth() date.Date
	YearOfBirth() int
	YearOfBirthAbs() int
	DateOfDeath() date.Date
	YearOfDeath() int
	YearOfDeathAbs() int
	Image() string
	Age() int
	Website() string
	WikipediaLink(language string) string
	SourceID() string
	RetrievedOn() time.Time
}

type DataSource interface {
	SearchAuthor(name string, language string) (Author, error)
	RetrieveAuthor(ID, language string) (Author, error)
}
