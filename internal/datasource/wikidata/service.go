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
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/webserver/controller/author"
)

const imgUrl = "https://commons.wikimedia.org/w/index.php?title=Special:Redirect/file/%s"

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

	entitiesReq, err := a.wikidata.NewGetEntities([]string{id})
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

	if value, exists := (*entities)[id].Claims[propertyInstanceOf]; exists {
		switch value[0].MainSnak.DataValue.Value.ValueFields.ID {
		case instanceOfHuman:
			author.instanceOf = InstanceHuman
		case instanceOfPseudonym:
			author.instanceOf = InstancePseudonym
		case instanceOfPenName:
			author.instanceOf = InstancePenName
		case instanceOfCollectivePseudonym:
			author.instanceOf = InstanceCollectivePseudonym
		default:
			return author, fmt.Errorf("instance of %s not supported", value[0].MainSnak.DataValue.Value.ValueFields.ID)
		}
	}

	if value, exists := (*entities)[id].Claims[propertySexOrGender]; exists {
		switch value[0].MainSnak.DataValue.Value.ValueFields.ID {
		case genderMale:
			author.gender = GenderMale
		case genderFemale:
			author.gender = GenderFemale
		case genderIntersex:
			author.gender = GenderIntersex
		case genderTrasgenderMale:
			author.gender = GenderTrasgenderMale
		case genderTrasgenderFemale:
			author.gender = GenderTrasgenderFemale
		default:
			return author, fmt.Errorf("gender %s not supported", value[0].MainSnak.DataValue.Value.ValueFields.ID)
		}
	}

	author.wikidataEntityId = id
	author.retrievedOn = time.Now()
	author.wikipediaLink[language] = (*entities)[id].SiteLinks[fmt.Sprintf("%swiki", language)].URL

	author.description[language] = (*entities)[id].Descriptions[language].Value
	if value, exists := (*entities)[id].Claims[propertyDateOfBirth]; exists {
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
	if value, exists := (*entities)[id].Claims[propertyDateOfDeath]; exists {
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
	if value, exists := (*entities)[id].Claims[propertyWebsite]; exists {
		author.website = value[0].MainSnak.DataValue.Value.S
	}
	if value, exists := (*entities)[id].Claims[propertyImage]; exists {
		img, err := strconv.Unquote("\"" + value[0].MainSnak.DataValue.Value.S + "\"")
		if err != nil {
			return author, err
		}

		if slices.Contains([]string{".png", ".jpg", ".jpeg"}, strings.ToLower(filepath.Ext(img))) {
			author.image = fmt.Sprintf(imgUrl, url.QueryEscape(img))
		}
	}

	return author, nil
}

func (a WikidataSource) getEntityId(name string) (string, error) {
	query, err := a.wikidata.NewSearch(url.QueryEscape(name), "en")
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
