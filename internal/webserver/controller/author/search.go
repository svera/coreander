package author

import (
	"log"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v4/internal/datasource/wikidata"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"github.com/svera/coreander/v4/internal/webserver/view"
)

func (a *Controller) Search(c fiber.Ctx) error {
	searchFields, err := a.parseAuthorSearchQuery(c)
	if err != nil {
		log.Println(err)
		return fiber.ErrBadRequest
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	var authorResults result.Paginated[[]index.Author]
	if authorResults, err = a.idx.SearchAuthors(searchFields, page, int(model.ResultsPerPage)); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	documentCounts := map[string]uint64{}
	if slugs := authorSlugs(authorResults.Hits()); len(slugs) > 0 {
		if documentCounts, err = a.idx.DocumentCountsByAuthorSlugs(slugs); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
	}

	templateVars := fiber.Map{
		"SearchFields":      searchFields,
		"SelectedGender":    c.Query("gender"),
		"Results":           authorResults,
		"DocumentCounts":    documentCounts,
		"Paginator":         view.Pagination(model.MaxPagesNavigator, authorResults, c.Queries()),
		"Title":             "Search authors",
		"AuthorsSearchPage": true,
		"URL":               view.URL(c),
		"SortURL":           view.BaseURLWithout(c, "sort-by", "page"),
		"SortBy":            c.Query("sort-by"),
		"AdditionalSortOptions": []struct {
			Key   string
			Value string
		}{
			{"name-a-z", "name A-Z"},
			{"name-z-a", "name Z-A"},
			{"birth-older-first", "birth older first"},
			{"birth-newer-first", "birth newer first"},
			{"death-older-first", "death older first"},
			{"death-newer-first", "death newer first"},
			{"documents-more-first", "documents more first"},
			{"documents-fewer-first", "documents fewer first"},
		},
	}

	if c.Get("hx-request") == "true" {
		if err = c.Render("partials/authors-list-fragments", templateVars); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		return nil
	}

	if err = c.Render("author/search", templateVars, "layout"); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return nil
}

func (a *Controller) parseAuthorSearchQuery(c fiber.Ctx) (index.AuthorSearchFields, error) {
	searchFields := index.AuthorSearchFields{
		Name:   c.Query("name"),
		SortBy: a.parseAuthorSearchSortBy(c),
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

func (a *Controller) parseAuthorSearchSortBy(c fiber.Ctx) []string {
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

func authorSlugs(authors []index.Author) []string {
	slugs := make([]string, len(authors))
	for i, author := range authors {
		slugs[i] = author.Slug
	}
	return slugs
}
