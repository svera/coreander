package precisiondate_test

import (
	"testing"
	"time"

	"github.com/svera/coreander/v4/internal/precisiondate"
)

func TestPrecisionDates(t *testing.T) {
	for _, tcase := range testCases() {
		t.Run(tcase.name, func(t *testing.T) {
			date := precisiondate.NewPrecisionDate(tcase.date, tcase.precision)

			if tcase.precision == precisiondate.PrecisionCentury && date.Century() != tcase.expectedCentury {
				t.Errorf("expected century %d, got %d", tcase.expectedCentury, date.Century())
			}

			if tcase.precision == precisiondate.PrecisionYear {
				if date.Century() != tcase.expectedCentury {
					t.Errorf("expected century %d, got %d", tcase.expectedCentury, date.Century())
				}
				if date.Year() != tcase.expectedYear {
					t.Errorf("expected year %d, got %d", tcase.expectedYear, date.Year())
				}
			}

			if tcase.precision == precisiondate.PrecisionDay {
				if date.Century() != tcase.expectedCentury {
					t.Errorf("expected century %d, got %d", tcase.expectedCentury, date.Century())
				}
				if date.Year() != tcase.expectedYear {
					t.Errorf("expected year %d, got %d", tcase.expectedYear, date.Year())
				}
				if date.Month() != tcase.expectedMonth {
					t.Errorf("expected month %d, got %d", tcase.expectedMonth, date.Month())
				}
				if date.Day() != tcase.expectedDay {
					t.Errorf("expected day %d, got %d", tcase.expectedDay, date.Day())
				}
			}

			// Test precision methods
			if tcase.precision == precisiondate.PrecisionCentury && !date.IsPrecisionCentury() {
				t.Error("expected IsPrecisionCentury to return true")
			}
			if tcase.precision == precisiondate.PrecisionDecade && !date.IsPrecisionDecade() {
				t.Error("expected IsPrecisionDecade to return true")
			}
			if tcase.precision == precisiondate.PrecisionYear && !date.IsPrecisionYear() {
				t.Error("expected IsPrecisionYear to return true")
			}
			if tcase.precision == precisiondate.PrecisionMonth && !date.IsPrecisionMonth() {
				t.Error("expected IsPrecisionMonth to return true")
			}
			if tcase.precision == precisiondate.PrecisionDay && !date.IsPrecisionDay() {
				t.Error("expected IsPrecisionDay to return true")
			}

			// Test CenturyAbs
			expectedAbsCentury := tcase.expectedCentury
			if expectedAbsCentury < 0 {
				expectedAbsCentury = -expectedAbsCentury
			}
			if date.CenturyAbs() != expectedAbsCentury {
				t.Errorf("expected CenturyAbs %d, got %d", expectedAbsCentury, date.CenturyAbs())
			}
		})
	}
}

func TestInvalidDates(t *testing.T) {
	invalidCases := []struct {
		name      string
		date      string
		precision float64
	}{
		{
			name:      "Invalid date format",
			date:      "invalid-date",
			precision: precisiondate.PrecisionDay,
		},
		{
			name:      "Invalid month",
			date:      "2024-13-01T00:00:00Z",
			precision: precisiondate.PrecisionDay,
		},
		{
			name:      "Invalid day",
			date:      "2024-02-30T00:00:00Z",
			precision: precisiondate.PrecisionDay,
		},
		{
			name:      "Invalid precision",
			date:      "2024-02-01T00:00:00Z",
			precision: 999,
		},
		{
			name:      "Empty date",
			date:      "",
			precision: precisiondate.PrecisionDay,
		},
		{
			name:      "Invalid year format",
			date:      "20-02-01T00:00:00Z",
			precision: precisiondate.PrecisionDay,
		},
	}

	for _, tcase := range invalidCases {
		t.Run(tcase.name, func(t *testing.T) {
			date := precisiondate.NewPrecisionDate(tcase.date, tcase.precision)
			if date.Date != 0 {
				t.Errorf("expected zero date for invalid input, got %v", date.Date)
			}
		})
	}
}

func TestEdgeCases(t *testing.T) {
	edgeCases := []struct {
		name            string
		date            string
		precision       float64
		expectedCentury int
		expectedYear    int
		expectedMonth   time.Month
		expectedDay     int
	}{
		{
			name:            "Year 0",
			date:            "+0000-01-01T00:00:00Z",
			precision:       precisiondate.PrecisionYear,
			expectedCentury: 1,
			expectedYear:    0,
		},
		{
			name:            "Year 1",
			date:            "+0001-01-01T00:00:00Z",
			precision:       precisiondate.PrecisionYear,
			expectedCentury: 1,
			expectedYear:    1,
		},
		{
			name:            "Year -1",
			date:            "-0001-01-01T00:00:00Z",
			precision:       precisiondate.PrecisionYear,
			expectedCentury: -1,
			expectedYear:    -1,
		},
		{
			name:            "Year 100",
			date:            "+0100-01-01T00:00:00Z",
			precision:       precisiondate.PrecisionYear,
			expectedCentury: 1,
			expectedYear:    100,
		},
		{
			name:            "Year 101",
			date:            "+0101-01-01T00:00:00Z",
			precision:       precisiondate.PrecisionYear,
			expectedCentury: 2,
			expectedYear:    101,
		},
		{
			name:            "Year -100",
			date:            "-0100-01-01T00:00:00Z",
			precision:       precisiondate.PrecisionYear,
			expectedCentury: -1,
			expectedYear:    -100,
		},
		{
			name:            "Year -101",
			date:            "-0101-01-01T00:00:00Z",
			precision:       precisiondate.PrecisionYear,
			expectedCentury: -2,
			expectedYear:    -101,
		},
		{
			name:            "Leap year",
			date:            "+2024-02-29T00:00:00Z",
			precision:       precisiondate.PrecisionDay,
			expectedCentury: 21,
			expectedYear:    2024,
			expectedMonth:   2,
			expectedDay:     29,
		},
	}

	for _, tcase := range edgeCases {
		t.Run(tcase.name, func(t *testing.T) {
			date := precisiondate.NewPrecisionDate(tcase.date, tcase.precision)

			if date.Century() != tcase.expectedCentury {
				t.Errorf("expected century %d, got %d", tcase.expectedCentury, date.Century())
			}

			if tcase.precision >= precisiondate.PrecisionYear {
				if date.Year() != tcase.expectedYear {
					t.Errorf("expected year %d, got %d", tcase.expectedYear, date.Year())
				}
			}

			if tcase.precision >= precisiondate.PrecisionDay {
				if date.Month() != tcase.expectedMonth {
					t.Errorf("expected month %d, got %d", tcase.expectedMonth, date.Month())
				}
				if date.Day() != tcase.expectedDay {
					t.Errorf("expected day %d, got %d", tcase.expectedDay, date.Day())
				}
			}
		})
	}
}

type testCase struct {
	name            string
	date            string
	precision       float64
	expectedCentury int
	expectedYear    int
	expectedMonth   time.Month
	expectedDay     int
}

func testCases() []testCase {
	return []testCase{
		{
			name:            "Lao Tse's birthday (century precision)",
			date:            "-0579-00-00T00:00:00Z",
			precision:       precisiondate.PrecisionCentury,
			expectedCentury: -6,
		},
		{
			name:            "Seneca's birthday (year precision)",
			date:            "-0004-00-00T00:00:00Z",
			precision:       precisiondate.PrecisionYear,
			expectedCentury: -1,
			expectedYear:    -4,
		},
		{
			name:            "Cervantes' birthday (day precision)",
			date:            "+1547-09-29T00:00:00Z",
			precision:       precisiondate.PrecisionDay,
			expectedCentury: 16,
			expectedYear:    1547,
			expectedMonth:   9,
			expectedDay:     29,
		},
		{
			name:            "Raiders' premiere (day precision without sign prefix)",
			date:            "1981-06-12T00:00:00Z",
			precision:       precisiondate.PrecisionDay,
			expectedCentury: 20,
			expectedYear:    1981,
			expectedMonth:   6,
			expectedDay:     12,
		},
		{
			name:            "Decade precision",
			date:            "1980-00-00T00:00:00Z",
			precision:       precisiondate.PrecisionDecade,
			expectedCentury: 20,
			expectedYear:    1980,
		},
		{
			name:            "Month precision",
			date:            "1981-06-00T00:00:00Z",
			precision:       precisiondate.PrecisionMonth,
			expectedCentury: 20,
			expectedYear:    1981,
			expectedMonth:   6,
		},
		{
			name:            "Negative century",
			date:            "-0200-00-00T00:00:00Z",
			precision:       precisiondate.PrecisionCentury,
			expectedCentury: -2,
		},
		{
			name:            "Positive century with plus sign",
			date:            "+0200-00-00T00:00:00Z",
			precision:       precisiondate.PrecisionCentury,
			expectedCentury: 2,
		},
	}
}
