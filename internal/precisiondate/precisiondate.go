package precisiondate

import (
	"fmt"
	"strings"

	"github.com/rickb777/date/v2"
)

// PrecisionDate implement a format for partially-known dates following Wikidata format.
// Some dates for relevant people are not fully known, for example, for Seneca, full
// birth and death dates are unknown, only the years.
// https://www.wikidata.org/wiki/Help:Dates
type PrecisionDate struct {
	date.Date
	Precision float64
}

const (
	PrecisionCentury = 7
	PrecisionDecade  = 8
	PrecisionYear    = 9
	PrecisionMonth   = 10
	PrecisionDay     = 11
)

// NewPrecisionDate parses a ISO 8601 formatted string and parses it into a PrecisionDate.
// As Wikidata dates come in different precision levels, we need to know what's the precision a date is using
// before parsing it, because dates in levels under day precision come with zero values
// for months or days, for example 2006-00-00.
// As this is not a valid ISO 8601, we need to convert it to a valid string beforehand.
func NewPrecisionDate(ISOdate string, precision float64) PrecisionDate {
	if precision < PrecisionDay {
		switch precision {
		case PrecisionDecade, PrecisionYear, PrecisionCentury:
			year := ISOdate[:4]
			if strings.HasPrefix(ISOdate, "-") || strings.HasPrefix(ISOdate, "+") {
				year = ISOdate[:5]
			}
			ISOdate = fmt.Sprintf("%s-01-01T00:00:00Z", year)
		case PrecisionMonth:
			yearMonth := ISOdate[:7]
			if strings.HasPrefix(ISOdate, "-") || strings.HasPrefix(ISOdate, "+") {
				yearMonth = ISOdate[:8]
			}
			ISOdate = fmt.Sprintf("%s-01T00:00:00Z", yearMonth)
		}
	}
	parsedDate, err := date.ParseISO(ISOdate)
	if err != nil {
		parsedDate = date.Zero
	}

	return PrecisionDate{
		Date:      parsedDate,
		Precision: precision,
	}
}

func (p PrecisionDate) IsPrecisionCentury() bool {
	return p.Precision == PrecisionCentury
}

func (p PrecisionDate) IsPrecisionDecade() bool {
	return p.Precision == PrecisionDecade
}

func (p PrecisionDate) IsPrecisionYear() bool {
	return p.Precision == PrecisionYear
}

func (p PrecisionDate) IsPrecisionMonth() bool {
	return p.Precision == PrecisionMonth
}

func (p PrecisionDate) IsPrecisionDay() bool {
	return p.Precision == PrecisionDay
}

func (p PrecisionDate) Century() int {
	if p.Date.Year() < 0 {
		return int(p.Date.Year()/100) - 1
	}
	return int(p.Date.Year()/100) + 1
}

func (p PrecisionDate) CenturyAbs() int {
	if p.Century() < 0 {
		return -p.Century()
	}
	return p.Century()
}
