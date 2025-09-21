package index

import (
	"strings"
	"time"

	"github.com/rickb777/date/v2"
	"github.com/rickb777/date/v2/timespan"
	"github.com/svera/coreander/v4/internal/precisiondate"
)

type Author struct {
	Slug            string
	Name            string
	BirthName       string
	DataSourceID    string
	RetrievedOn     time.Time
	Type            string
	WikipediaLink   map[string]string
	InstanceOf      float64
	Description     map[string]string
	DateOfBirth     precisiondate.PrecisionDate
	DateOfDeath     precisiondate.PrecisionDate
	Website         string
	DataSourceImage string
	Gender          float64
	Pseudonyms      []string
}

// BleveType is part of the bleve.Classifier interface and its purpose is to tell the indexer
// the type of the document, which will be used to decide which analyzer will parse it.
func (a Author) BleveType() string {
	return "author"
}

func (a Author) YearOfBirthAbs() int {
	if a.DateOfBirth.Year() < 0 {
		return -a.DateOfBirth.Year()
	}
	return a.DateOfBirth.Year()
}

func (a Author) YearOfDeathAbs() int {
	if a.DateOfDeath.Year() < 0 {
		return -a.DateOfDeath.Year()
	}
	return a.DateOfDeath.Year()
}

func (a Author) Age() int {
	if a.DateOfBirth.Date == 0 {
		return 0
	}

	if a.DateOfBirth.Precision < precisiondate.PrecisionDay {
		return 0
	}

	period := timespan.BetweenDates(a.DateOfBirth.Date, date.Today())
	if a.DateOfDeath.Date != 0 {
		if a.DateOfDeath.Precision < precisiondate.PrecisionDay {
			return 0
		}
		period = timespan.BetweenDates(a.DateOfBirth.Date, a.DateOfDeath.Date)
	}

	return int(float64(period.Days()) / 365.25)
}

func (a Author) BirthNameIncludesName() bool {
	for part := range strings.SplitSeq(a.Name, " ") {
		if !strings.Contains(a.BirthName, part) {
			return false
		}
	}
	return true
}
