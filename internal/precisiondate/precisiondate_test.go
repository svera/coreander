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
	}
}
