package wikidata

import (
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v4/internal/webserver/controller/author"
)

const imgUrl = "https://commons.wikimedia.org/w/index.php?title=Special:Redirect/file/%s"

const (
	precisionCentury = 7
	precisionDecade  = 8
	precisionYear    = 9
	precisionMonth   = 10
	precisionDay     = 11
)

type wikidata interface {
	NewSearch(string, string) (SearchEntitiesRequest, error)
	NewGetEntities([]string) (GetEntitiesRequest, error)
}

type SearchEntitiesRequest interface {
	Get() (*gowikidata.SearchEntitiesResponse, error)
}

type GetEntitiesRequest interface {
	SetProps([]string)
	SetLanguages([]string)
	Get() (*map[string]gowikidata.Entity, error)
}

type WikidataSource struct {
	wikidata wikidata
}

func NewWikidataSource(w wikidata) WikidataSource {
	return WikidataSource{w}
}

func (a WikidataSource) SearchAuthor(name string, languages []string) (author.Author, error) {
	ids, err := a.getEntityIds(name)
	if err != nil {
		return nil, err
	}

	return a.RetrieveAuthor(ids, languages)
}

// RetrieveAuthor returns the first match from the list of passed Wikidata entity IDs that represents a human
func (a WikidataSource) RetrieveAuthor(ids []string, languages []string) (author.Author, error) {
	author := Author{
		name:          make(map[string]string),
		wikipediaLink: make(map[string]string),
		description:   make(map[string]string),
	}

	entitiesReq, err := a.wikidata.NewGetEntities(ids)
	if err != nil {
		return nil, err
	}
	entitiesReq.SetProps([]string{"descriptions", "claims", "sitelinks/urls", "labels"})
	entitiesReq.SetLanguages(languages)
	// Call get to make the request based on the configurations
	entities, err := entitiesReq.Get()
	if err != nil {
		return nil, err
	}

	id := ""
	for _, id = range ids {
		if instanceOf, exists := (*entities)[id].Claims[propertyInstanceOf]; exists {
			if parseInstanceOf(instanceOf[0]) != InstanceUnknown {
				break
			}
		}
	}

	if id == "" {
		return author, nil
	}

	if value, exists := (*entities)[id].Claims[propertyBirthName]; exists {
		author.birthName = value[0].MainSnak.DataValue.Value.ValueFields.Text
	} else if value, exists := (*entities)[id].Claims[propertyNameInNativeLanguage]; exists {
		author.birthName = value[0].MainSnak.DataValue.Value.ValueFields.Text
	} else if value, exists := (*entities)[id].Claims[propertyOfficialName]; exists {
		author.birthName = value[0].MainSnak.DataValue.Value.ValueFields.Text
	}

	if value, exists := (*entities)[id].Claims[propertyInstanceOf]; exists {
		author.instanceOf = parseInstanceOf(value[0])
	}

	if value, exists := (*entities)[id].Claims[propertySexOrGender]; exists {
		author.gender = parseGender(value[0])
	}

	author.wikidataEntityId = id
	author.retrievedOn = time.Now().UTC()
	for _, lang := range languages {
		author.name[lang] = (*entities)[id].Labels[lang].Value
		author.wikipediaLink[lang] = (*entities)[id].SiteLinks[fmt.Sprintf("%swiki", lang)].URL
		author.description[lang] = (*entities)[id].Descriptions[lang].Value
	}
	if value, exists := (*entities)[id].Claims[propertyDateOfBirth]; exists {
		author.yearOfBirth, author.dateOfBirth = parseDateProperty(value[0])
	}
	if value, exists := (*entities)[id].Claims[propertyDateOfDeath]; exists {
		author.yearOfDeath, author.dateOfDeath = parseDateProperty(value[0])
	}
	if value, exists := (*entities)[id].Claims[propertyWebsite]; exists {
		author.website = value[0].MainSnak.DataValue.Value.S
	}
	if value, exists := (*entities)[id].Claims[propertyPseudonym]; exists {
		author.pseudonyms = make([]string, 0, len(value))
		for _, claim := range value {
			pseudonym, err := strconv.Unquote("\"" + claim.MainSnak.DataValue.Value.S + "\"")
			if err != nil {
				continue
			}
			author.pseudonyms = append(author.pseudonyms, pseudonym)
		}
	}

	if value, exists := (*entities)[id].Claims[propertyImage]; exists {
		img, err := strconv.Unquote("\"" + value[0].MainSnak.DataValue.Value.S + "\"")
		if err != nil {
			return nil, err
		}

		if slices.Contains([]string{".png", ".jpg", ".jpeg"}, strings.ToLower(filepath.Ext(img))) {
			author.image = fmt.Sprintf(imgUrl, url.QueryEscape(img))
		}
	}

	return author, nil
}

// getEntityIds return all entity IDs from Wikidata which matches the passed name
func (a WikidataSource) getEntityIds(name string) ([]string, error) {
	query, err := a.wikidata.NewSearch(url.QueryEscape(name), "en")
	if err != nil {
		return []string{}, err
	}
	result, err := query.Get()
	if err != nil {
		return []string{}, err
	}

	if len(result.SearchResult) == 0 {
		return []string{}, fmt.Errorf("no entity found for %s", name)
	}

	res := make([]string, 0, len(result.SearchResult))
	for _, entity := range result.SearchResult {
		res = append(res, entity.ID)
	}

	return res, nil
}

func parseDateProperty(claim gowikidata.Claim) (int, date.Date) {
	if claim.MainSnak.DataValue.Value.ValueFields.Precision == precisionYear {
		year, err := strconv.Atoi(claim.MainSnak.DataValue.Value.ValueFields.Time[:5])
		if err != nil {
			return 0, date.Zero
		}
		return year, date.Zero
	}
	parsedDate, err := date.ParseISO(claim.MainSnak.DataValue.Value.ValueFields.Time)
	if err != nil {
		return 0, date.Zero
	}
	return 0, parsedDate
}

func parseGender(claim gowikidata.Claim) int {
	switch claim.MainSnak.DataValue.Value.ValueFields.ID {
	case qidGenderMale:
		return GenderMale
	case qidGenderFemale:
		return GenderFemale
	case qidGenderIntersex:
		return GenderIntersex
	case qidGenderTrasgenderMale:
		return GenderTrasgenderMale
	case qidGenderTrasgenderFemale:
		return GenderTrasgenderFemale
	}
	return GenderUnknown
}

func parseInstanceOf(claim gowikidata.Claim) int {
	switch claim.MainSnak.DataValue.Value.ValueFields.ID {
	case qidInstanceOfHuman:
		return InstanceHuman
	case qidInstanceOfPseudonym:
		return InstancePseudonym
	case qidInstanceOfPenName:
		return InstancePenName
	case qidInstanceOfCollectivePseudonym:
		return InstanceCollectivePseudonym
	}
	return InstanceUnknown
}
