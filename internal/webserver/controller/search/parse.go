package search

import (
	"log"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v5/internal/datasource/wikidata"
	"github.com/svera/coreander/v5/internal/index"
)

func parseDocumentSearchQuery(c fiber.Ctx, wordsPerMinute float64) (index.SearchFields, error) {
	searchFields := index.SearchFields{
		Keywords:        c.Query("search"),
		Language:        c.Query("language"),
		Subjects:        c.Query("subjects"),
		SortBy:          parseDocumentSortBy(c),
		EstReadTimeFrom: fiber.Query[float64](c, "est-read-time-from", 0),
		EstReadTimeTo:   fiber.Query[float64](c, "est-read-time-to", 0),
		WordsPerMinute:  wordsPerMinute,
		IllustratedOnly: c.Query("illustrated-only") == "on" || c.Query("illustrated-only") == "1",
	}

	if c.Query("pub-date-from") != "" {
		pubDateFrom, err := date.ParseISO(c.Query("pub-date-from"))
		if err != nil {
			return searchFields, err
		}
		searchFields.PubDateFrom = pubDateFrom
	}

	if c.Query("pub-date-to") != "" {
		pubDateTo, err := date.ParseISO(c.Query("pub-date-to"))
		if err != nil {
			return searchFields, err
		}
		searchFields.PubDateTo = pubDateTo
	}

	if searchFields.PubDateTo != 0 && searchFields.PubDateFrom > searchFields.PubDateTo {
		searchFields.PubDateFrom, searchFields.PubDateTo = searchFields.PubDateTo, searchFields.PubDateFrom
	}

	if searchFields.EstReadTimeTo != 0 && searchFields.EstReadTimeFrom > searchFields.EstReadTimeTo {
		searchFields.EstReadTimeFrom, searchFields.EstReadTimeTo = searchFields.EstReadTimeTo, searchFields.EstReadTimeFrom
	}

	return searchFields, nil
}

func parseAuthorSearchQuery(c fiber.Ctx) (index.AuthorSearchFields, error) {
	name := c.Query("name")
	if name == "" {
		name = c.Query("search")
	}

	searchFields := index.AuthorSearchFields{
		Name:   name,
		SortBy: parseAuthorSortBy(c),
	}

	if gender, ok := parseGenderQuery(c.Query("gender")); ok {
		searchFields.Gender = &gender
	}

	if c.Query("birthdate-from") != "" {
		birthDateFrom, err := date.ParseISO(c.Query("birthdate-from"))
		if err != nil {
			return searchFields, err
		}
		searchFields.BirthDateFrom = birthDateFrom
	}

	if c.Query("birthdate-to") != "" {
		birthDateTo, err := date.ParseISO(c.Query("birthdate-to"))
		if err != nil {
			return searchFields, err
		}
		searchFields.BirthDateTo = birthDateTo
	}

	if c.Query("deathdate-from") != "" {
		deathDateFrom, err := date.ParseISO(c.Query("deathdate-from"))
		if err != nil {
			return searchFields, err
		}
		searchFields.DeathDateFrom = deathDateFrom
	}

	if c.Query("deathdate-to") != "" {
		deathDateTo, err := date.ParseISO(c.Query("deathdate-to"))
		if err != nil {
			return searchFields, err
		}
		searchFields.DeathDateTo = deathDateTo
	}

	if searchFields.BirthDateTo != 0 && searchFields.BirthDateFrom > searchFields.BirthDateTo {
		searchFields.BirthDateFrom, searchFields.BirthDateTo = searchFields.BirthDateTo, searchFields.BirthDateFrom
	}
	if searchFields.DeathDateTo != 0 && searchFields.DeathDateFrom > searchFields.DeathDateTo {
		searchFields.DeathDateFrom, searchFields.DeathDateTo = searchFields.DeathDateTo, searchFields.DeathDateFrom
	}

	return searchFields, nil
}

func parseGenderQuery(value string) (float64, bool) {
	switch value {
	case "male":
		return wikidata.GenderMale, true
	case "female":
		return wikidata.GenderFemale, true
	case "intersex":
		return wikidata.GenderIntersex, true
	case "transgender-female":
		return wikidata.GenderTrasgenderFemale, true
	case "transgender-male":
		return wikidata.GenderTrasgenderMale, true
	case "unknown":
		return wikidata.GenderUnknown, true
	default:
		return 0, false
	}
}

func parseDocumentSortBy(c fiber.Ctx) []string {
	if c.Query("sort-by") != "" {
		switch c.Query("sort-by") {
		case "pub-date-older-first":
			return []string{"Publication.Date"}
		case "pub-date-newer-first":
			return []string{"-Publication.Date"}
		case "est-read-time-shorter-first":
			return []string{"Words"}
		case "est-read-time-longer-first":
			return []string{"-Words"}
		}
	}
	return []string{"-_score", "Series", "SeriesIndex"}
}

func parseAuthorSortBy(c fiber.Ctx) []string {
	switch c.Query("sort-by") {
	case "name-z-a":
		return []string{"-Name"}
	case "birth-older-first":
		return []string{"DateOfBirth.Date"}
	case "birth-newer-first":
		return []string{"-DateOfBirth.Date"}
	case "death-older-first":
		return []string{"DateOfDeath.Date"}
	case "death-newer-first":
		return []string{"-DateOfDeath.Date"}
	case "documents-more-first":
		return []string{"-DocumentCount"}
	case "documents-fewer-first":
		return []string{"DocumentCount"}
	default:
		return []string{"Name"}
	}
}

func searchTypeFromContext(c fiber.Ctx) string {
	if val, ok := c.Locals("SearchType").(string); ok && val != "" {
		return val
	}
	if t := c.Query("type"); t == TypeAuthors {
		return TypeAuthors
	}
	return TypeDocuments
}

func parsePage(c fiber.Ctx) int {
	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		return 1
	}
	return page
}

func authorSlugs(authors []index.Author) []string {
	slugs := make([]string, len(authors))
	for i, author := range authors {
		slugs[i] = author.Slug
	}
	return slugs
}

// Subjects returns all subjects from the index grouped by slug, as JSON.
func (s *Controller) Subjects(c fiber.Ctx) error {
	bySlug, err := s.idx.Subjects()
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return c.JSON(bySlug)
}
