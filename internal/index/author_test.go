package index_test

import (
	"testing"

	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/precisiondate"
)

func TestAge(t *testing.T) {
	for _, tcase := range testCasesAuthorAge() {
		t.Run(tcase.name, func(t *testing.T) {
			if age, expectedAge := tcase.author.Age(), tcase.expectedAge; age != expectedAge {
				t.Errorf("Wrong author age, expected '%d', got '%d'", age, expectedAge)
			}
		})
	}
}

type testCaseAuthorAge struct {
	name        string
	author      index.Author
	expectedAge int
}

func testCasesAuthorAge() []testCaseAuthorAge {
	return []testCaseAuthorAge{
		{
			name: "Leigh Brackett",
			author: index.Author{
				DateOfBirth: precisiondate.NewPrecisionDate(
					"+1915-12-07T00:00:00Z",
					precisiondate.PrecisionDay,
				),
				DateOfDeath: precisiondate.NewPrecisionDate(
					"+1978-03-18T00:00:00Z",
					precisiondate.PrecisionDay,
				),
			},
			expectedAge: 62,
		},
		{
			name: "Lucius Annaeus Seneca (not enough precision in date of birth)",
			author: index.Author{
				DateOfBirth: precisiondate.NewPrecisionDate(
					"-0004-00-00T00:00:00Z",
					precisiondate.PrecisionYear,
				),
				DateOfDeath: precisiondate.NewPrecisionDate(
					"+0065-04-12T00:00:00Z",
					precisiondate.PrecisionDay,
				),
			},
			expectedAge: 0,
		},
	}
}

func TestBirthNameIncludesName(t *testing.T) {
	for _, tcase := range testCasesAuthorBirthNameIncludesName() {
		t.Run(tcase.name, func(t *testing.T) {
			if result, expectedResult := tcase.author.BirthNameIncludesName(), tcase.expectedResult; result != expectedResult {
				t.Errorf("Wrong result in birth name includes name, expected '%v', got '%v'", result, expectedResult)
			}
		})
	}
}

type testCaseAuthorBirthNameIncludesName struct {
	name           string
	author         index.Author
	expectedResult bool
}

func testCasesAuthorBirthNameIncludesName() []testCaseAuthorBirthNameIncludesName {
	return []testCaseAuthorBirthNameIncludesName{
		{
			name: "Arturo Pérez-Reverte Gutiérrez",
			author: index.Author{
				Name:      "Arturo Pérez-Reverte",
				BirthName: "Arturo Pérez-Reverte Gutiérrez",
			},
			expectedResult: true,
		},
		{
			name: "George Orwell",
			author: index.Author{
				Name:      "George Orwell",
				BirthName: "Eric Arthur Blair",
			},
			expectedResult: false,
		},
	}
}
