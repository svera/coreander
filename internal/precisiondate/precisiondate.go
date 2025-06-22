package precisiondate

import (
	"fmt"
	"strconv"
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

// validatePrecision checks if the precision value is valid
func validatePrecision(precision float64) bool {
	return precision >= PrecisionCentury && precision <= PrecisionDay
}

// validateDate checks if the date string is valid for the given precision
func validateDate(ISOdate string, precision float64) bool {
	if ISOdate == "" {
		return false
	}

	// Check if the date has a valid format
	parts := strings.Split(ISOdate, "T")
	if len(parts) != 2 {
		return false
	}

	datePart := parts[0]
	timePart := parts[1]

	// Validate time part
	if !strings.HasSuffix(timePart, "Z") {
		return false
	}

	// Parse date part
	var yearStr string
	if strings.HasPrefix(datePart, "-") || strings.HasPrefix(datePart, "+") {
		yearStr = datePart[:5]
	} else {
		yearStr = datePart[:4]
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil {
		return false
	}

	// For century precision, we only need to validate the year
	if precision == PrecisionCentury {
		return true
	}

	// For decade and year precision, validate year format
	if precision <= PrecisionYear {
		return len(datePart) >= 4
	}

	// For month precision, validate month
	if precision == PrecisionMonth {
		parts := strings.Split(datePart, "-")
		if len(parts) < 2 {
			return false
		}
		month, err := strconv.Atoi(parts[1])
		if err != nil || month < 1 || month > 12 {
			return false
		}
		return true
	}

	// For day precision, validate the full date
	if precision == PrecisionDay {
		parts := strings.Split(datePart, "-")
		if len(parts) != 3 {
			return false
		}
		month, err := strconv.Atoi(parts[1])
		if err != nil || month < 1 || month > 12 {
			return false
		}
		day, err := strconv.Atoi(parts[2])
		if err != nil || day < 1 || day > 31 {
			return false
		}
		// Basic validation for days in month
		if month == 2 {
			if day > 29 {
				return false
			}
			if day == 29 {
				// Check for leap year
				if year%4 != 0 || (year%100 == 0 && year%400 != 0) {
					return false
				}
			}
		} else if month == 4 || month == 6 || month == 9 || month == 11 {
			if day > 30 {
				return false
			}
		}
		return true
	}

	return false
}

// NewPrecisionDate parses a ISO 8601 formatted string and parses it into a PrecisionDate.
// As Wikidata dates come in different precision levels, we need to know what's the precision a date is using
// before parsing it, because dates in levels under day precision come with zero values
// for months or days, for example 2006-00-00.
// As this is not a valid ISO 8601, we need to convert it to a valid string beforehand.
func NewPrecisionDate(ISOdate string, precision float64) PrecisionDate {
	if !validatePrecision(precision) || !validateDate(ISOdate, precision) {
		return PrecisionDate{Date: date.Zero, Precision: precision}
	}

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
		return PrecisionDate{Date: date.Zero, Precision: precision}
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
	year := p.Date.Year()
	if year == 0 {
		return 1
	}
	if year > 0 {
		return ((year - 1) / 100) + 1
	}
	return -(((-year - 1) / 100) + 1)
}

func (p PrecisionDate) CenturyAbs() int {
	if p.Century() < 0 {
		return -p.Century()
	}
	return p.Century()
}

// FormatForLocale returns an ISO format string for day precision dates, adapting to locale conventions.
func (p PrecisionDate) FormatForLocale(locale string) string {
	// Return appropriate format based on locale
	switch locale {
	case "en": // US/English format (YYYY-MM-DD)
		return p.Date.Format("2006-01-02")
	default: // European format (DD-MM-YYYY)
		return p.Date.Format("02-01-2006")
	}
}
