package index

import (
	"strings"
	"time"

	"github.com/rickb777/date/v2"
	"github.com/rickb777/date/v2/timespan"
)

type Author struct {
	Slug          string
	Name          string
	BirthName     string
	DataSourceID  string
	RetrievedOn   time.Time
	Type          string
	WikipediaLink map[string]string
	InstanceOf    int
	Description   map[string]string
	DateOfBirth   date.Date
	YearOfBirth   int // Used when DateOfBirth is not available
	DateOfDeath   date.Date
	YearOfDeath   int // Used when DateOfDeath is not available
	Website       string
	Image         string
	Gender        int
	Pseudonyms    []string
}

// BleveType is part of the bleve.Classifier interface and its purpose is to tell the indexer
// the type of the document, which will be used to decide which analyzer will parse it.
func (a Author) BleveType() string {
	return "author"
}

func (a Author) YearOfBirthAbs() int {
	if a.YearOfBirth < 0 {
		return -a.YearOfBirth
	}
	return a.YearOfBirth
}

func (a Author) YearOfDeathAbs() int {
	if a.YearOfDeath < 0 {
		return -a.YearOfDeath
	}
	return a.YearOfDeath
}

func (a Author) Age() int {
	if a.DateOfBirth == 0 {
		return 0
	}

	period := timespan.BetweenDates(a.DateOfBirth, date.Today())
	if a.DateOfDeath != 0 {
		period = timespan.BetweenDates(a.DateOfBirth, a.DateOfDeath)
	}

	return int(period.Days() / 365)
}

func (a Author) BirthNameIncludesName() bool {
	nameParts := strings.Split(a.Name, " ")
	for _, part := range nameParts {
		if !strings.Contains(a.BirthName, part) {
			return false
		}
	}
	return true
}
