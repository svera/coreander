package author

import "github.com/rickb777/date/v2"

type Author interface {
	Description(language string) string
	DateOfBirth() date.Date
	DateOfDeath() date.Date
	Website() string
	Image() string
	InstanceOfHuman() bool
	InstanceOfPseudonym() bool
	InstanceOfPenName() bool
	InstanceOfCollectivePseudonym() bool
	YearOfBirth() int
	YearOfBirthAbs() int
	YearOfDeathAbs() int
	YearOfDeath() int
	Age() int
	WikipediaLink(language string) string
}

type DataSource interface {
	Author(name, language string) (Author, error)
}
