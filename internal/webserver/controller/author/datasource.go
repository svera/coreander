package author

import (
	"time"

	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v4/internal/index"
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
	Author(author index.Author, language string) (Author, error)
	Retrieve(ID, language string) (Author, error)
}
