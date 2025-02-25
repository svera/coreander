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

func (a WikidataSource) SearchAuthor(name string, language string) (author.Author, error) {
	id, err := a.getEntityId(name)
	if err != nil {
		return nil, err
	}

	return a.RetrieveAuthor(id, language)
}

func (a WikidataSource) RetrieveAuthor(id string, language string) (author.Author, error) {
	author := Author{
		wikipediaLink: make(map[string]string),
		description:   make(map[string]string),
	}

	entitiesReq, err := a.wikidata.NewGetEntities([]string{id})
	if err != nil {
		return nil, err
	}
	entitiesReq.SetProps([]string{"descriptions", "claims", "sitelinks/urls"})
	entitiesReq.SetLanguages([]string{"en", language})
	// Call get to make the request based on the configurations
	entities, err := entitiesReq.Get()
	if err != nil {
		return nil, err
	}

	if value, exists := (*entities)[id].Claims[propertyInstanceOf]; exists {
		switch value[0].MainSnak.DataValue.Value.ValueFields.ID {
		case qidInstanceOfHuman:
			author.instanceOf = InstanceHuman
		case qidInstanceOfPseudonym:
			author.instanceOf = InstancePseudonym
		case qidInstanceOfPenName:
			author.instanceOf = InstancePenName
		case qidInstanceOfCollectivePseudonym:
			author.instanceOf = InstanceCollectivePseudonym
		default:
			return author, fmt.Errorf("instance of %s not supported", value[0].MainSnak.DataValue.Value.ValueFields.ID)
		}
	}

	if value, exists := (*entities)[id].Claims[propertySexOrGender]; exists {
		switch value[0].MainSnak.DataValue.Value.ValueFields.ID {
		case qidGenderMale:
			author.gender = GenderMale
		case qidGenderFemale:
			author.gender = GenderFemale
		case qidGenderIntersex:
			author.gender = GenderIntersex
		case qidGenderTrasgenderMale:
			author.gender = GenderTrasgenderMale
		case qidGenderTrasgenderFemale:
			author.gender = GenderTrasgenderFemale
		default:
			return author, fmt.Errorf("gender %s not supported", value[0].MainSnak.DataValue.Value.ValueFields.ID)
		}
	}

	author.wikidataEntityId = id
	author.retrievedOn = time.Now().UTC()
	author.wikipediaLink[language] = (*entities)[id].SiteLinks[fmt.Sprintf("%swiki", language)].URL

	author.description[language] = (*entities)[id].Descriptions[language].Value
	if value, exists := (*entities)[id].Claims[propertyDateOfBirth]; exists {
		if value[0].MainSnak.DataValue.Value.ValueFields.Precision == precisionYear {
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
		if value[0].MainSnak.DataValue.Value.ValueFields.Precision == precisionYear {
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
			return nil, err
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
		return "", fmt.Errorf("no author found for %s", name)
	}

	//result.SearchResult[0].Match.Type == "alias"
	return result.SearchResult[0].ID, nil
}
