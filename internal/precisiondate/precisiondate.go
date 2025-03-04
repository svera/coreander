package precisiondate

import (
	"fmt"

	"github.com/rickb777/date/v2"
)

// PrecisionDate implement a format for partially-known dates following Wikidata format
// Some dates for relevant people are not fully known, for example, for Seneca, full
// birth and death dates are not known, only the years.
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

func NewPrecisionDate(ISOdate string, precision float64) PrecisionDate {
	if precision < PrecisionDay {
		switch precision {
		case PrecisionDecade, PrecisionYear:
			year := ISOdate[:5]
			ISOdate = fmt.Sprintf("%s-01-01T00:00:00Z", year)
		case PrecisionMonth:
			yearMonth := ISOdate[:8]
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
