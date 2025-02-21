package wikidata

import (
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rickb777/date/v2"
	"github.com/rickb777/date/v2/timespan"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/webserver/controller/author"
)

const imgUrl = "https://commons.wikimedia.org/w/index.php?title=Special:Redirect/file/%s"

type Authordata struct {
	wikidataEntityId string
	wikipediaLink    map[string]string
	instanceOf       int
	description      map[string]string
	dateOfBirth      date.Date
	yearOfBirth      int // Used when dateOfBirth is not available
	dateOfDeath      date.Date
	yearOfDeath      int // Used when dateOfDeath is not available
	website          string
	image            string
	retrievedOn      time.Time
	gender           int
}

func (a Authordata) Description(language string) string {
	return a.description[language]
}

func (a Authordata) DateOfBirth() date.Date {
	return a.dateOfBirth
}

func (a Authordata) DateOfDeath() date.Date {
	return a.dateOfDeath
}

func (a Authordata) Website() string {
	return a.website
}

func (a Authordata) Image() string {
	return a.image
}

func (a Authordata) InstanceOf() int {
	return a.instanceOf
}

func (a Authordata) YearOfBirth() int {
	return a.yearOfBirth
}

func (a Authordata) YearOfBirthAbs() int {
	if a.yearOfBirth < 0 {
		return -a.yearOfBirth
	}
	return a.yearOfBirth
}

func (a Authordata) YearOfDeathAbs() int {
	if a.yearOfDeath < 0 {
		return -a.yearOfDeath
	}
	return a.yearOfDeath
}

func (a Authordata) YearOfDeath() int {
	return a.yearOfDeath
}

func (a Authordata) Age() int {
	if a.dateOfBirth == 0 {
		return 0
	}
	if a.dateOfDeath == 0 {
		period := timespan.BetweenDates(a.dateOfBirth, date.Today())
		return int(period.Days() / 365)
	}
	period := timespan.BetweenDates(a.dateOfBirth, a.dateOfDeath)
	return int(period.Days() / 365)
}

func (a Authordata) WikipediaLink(language string) string {
	return a.wikipediaLink[language]
}

func (a Authordata) Gender() int {
	return a.gender
}

func (a Authordata) SourceID() string {
	return a.wikidataEntityId
}

func (a Authordata) RetrievedOn() time.Time {
	return a.retrievedOn
}

type WikidataSource struct {
	wikidata
}

func NewWikidataSource(w wikidata) WikidataSource {
	return WikidataSource{w}
}

func (a WikidataSource) Author(author index.Author, language string) (author.Author, error) {
	if author.WikidataID != "" {
		return a.Retrieve(author.WikidataID, language)
	}

	id, err := a.getEntityId(author.Name)
	if err != nil {
		return Authordata{}, err
	}

	return a.Retrieve(id, language)
}

func (a WikidataSource) Retrieve(id string, language string) (author.Author, error) {
	author := Authordata{
		wikipediaLink: make(map[string]string),
		description:   make(map[string]string),
	}

	entitiesReq, err := a.NewGetEntities([]string{id})
	if err != nil {
		return author, err
	}
	entitiesReq.SetProps([]string{"descriptions", "claims", "sitelinks/urls"})
	entitiesReq.SetLanguages([]string{"en", language})
	// Call get to make the request based on the configurations
	entities, err := entitiesReq.Get()
	if err != nil {
		return author, err
	}

	if value, exists := (*entities)[id].Claims[claimInstanceOf]; exists {
		switch value[0].MainSnak.DataValue.Value.ValueFields.ID {
		case instanceOfHuman:
			author.instanceOf = index.InstanceHuman
		case instanceOfPseudonym:
			author.instanceOf = index.InstancePseudonym
		case instanceOfPenName:
			author.instanceOf = index.InstancePenName
		case instanceOfCollectivePseudonym:
			author.instanceOf = index.InstanceCollectivePseudonym
		default:
			return author, fmt.Errorf("instance of %s not supported", value[0].MainSnak.DataValue.Value.ValueFields.ID)
		}
	}

	if value, exists := (*entities)[id].Claims[claimSexOrGender]; exists {
		switch value[0].MainSnak.DataValue.Value.ValueFields.ID {
		case genderMale:
			author.gender = index.GenderMale
		case genderFemale:
			author.gender = index.GenderFemale
		case genderIntersex:
			author.gender = index.GenderIntersex
		case genderTrasgenderMale:
			author.gender = index.GenderTrasgenderMale
		case genderTrasgenderFemale:
			author.gender = index.GenderTrasgenderFemale
		default:
			return author, fmt.Errorf("gender %s not supported", value[0].MainSnak.DataValue.Value.ValueFields.ID)
		}
	}

	author.wikidataEntityId = id
	author.retrievedOn = time.Now()
	author.wikipediaLink[language] = (*entities)[id].SiteLinks[fmt.Sprintf("%swiki", language)].URL

	author.description[language] = (*entities)[id].Descriptions[language].Value
	if value, exists := (*entities)[id].Claims[claimDateOfBirth]; exists {
		if strings.Contains(value[0].MainSnak.DataValue.Value.ValueFields.Time, "00T") {
			author.yearOfBirth, err = strconv.Atoi(value[0].MainSnak.DataValue.Value.ValueFields.Time[:5])
			if err != nil {
				author.yearOfBirth = 0
				author.dateOfBirth = date.Zero
			}
		} else {
			author.dateOfBirth, err = date.ParseISO(value[0].MainSnak.DataValue.Value.ValueFields.Time)
			if err != nil {
				author.dateOfBirth = date.Zero
			}
		}
	}
	if value, exists := (*entities)[id].Claims[claimDateOfDeath]; exists {
		if strings.Contains(value[0].MainSnak.DataValue.Value.ValueFields.Time, "00T") {
			author.yearOfDeath, err = strconv.Atoi(value[0].MainSnak.DataValue.Value.ValueFields.Time[:5])
			if err != nil {
				author.yearOfDeath = 0
				author.dateOfDeath = date.Zero
			}
		} else {
			author.dateOfDeath, err = date.ParseISO(value[0].MainSnak.DataValue.Value.ValueFields.Time)
			if err != nil {
				author.dateOfBirth = date.Zero
			}
		}
	}
	if value, exists := (*entities)[id].Claims[claimWebsite]; exists {
		author.website = value[0].MainSnak.DataValue.Value.S
	}
	if value, exists := (*entities)[id].Claims[claimImage]; exists {
		img, err := strconv.Unquote("\"" + value[0].MainSnak.DataValue.Value.S + "\"")
		if err != nil {
			return author, err
		}

		if slices.Contains([]string{".png", ".jpg", ".jpeg"}, strings.ToLower(filepath.Ext(img))) {
			author.image = fmt.Sprintf(imgUrl, img)
		}
	}

	return author, nil
}

func (a WikidataSource) getEntityId(name string) (string, error) {
	query, err := a.NewSearch(url.QueryEscape(name), "en")
	if err != nil {
		return "", err
	}
	result, err := query.Get()
	if err != nil {
		return "", err
	}

	if len(result.SearchResult) == 0 {
		return "", fmt.Errorf("no author found")
	}

	//result.SearchResult[0].Match.Type == "alias"
	return result.SearchResult[0].ID, nil
}
