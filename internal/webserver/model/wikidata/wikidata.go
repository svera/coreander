package wikidata

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	gowikidata "github.com/Navid2zp/go-wikidata"
)

const imgUrl = "https://commons.wikimedia.org/w/index.php?title=Special:Redirect/file/%s"

const (
	claimInstanceOf  = "P31"
	claimImage       = "P18"
	claimSexOrGender = "P21"
	claimDateOfBirth = "P569"
	claimDateOfDeath = "P570"
	claimWebsite     = "P856"
)

const (
	instanceOfHuman               = "Q5"
	instanceOfPseudonym           = "Q61002"
	instanceOfPenName             = "Q127843"
	instanceOfCollectivePseudonym = "Q16017119"
)

const (
	isHuman = iota
	isPseudonym
	isPenName
	isCollectivePseudonym
)

const (
	male             = "Q6581097"
	female           = "Q6581072"
	intersex         = "Q1097630"
	trasgenderFemale = "Q1052281"
	trasgenderMale   = "Q2449503"
)

type Authordata struct {
	wikidataEntityId string
	instanceOf       int
	description      string
	dateOfBirth      time.Time
	dateOfDeath      time.Time
	website          string
	image            string
	retrievedOn      time.Time
}

func (a Authordata) Description() string {
	return a.description
}

func (a Authordata) DateOfBirth() time.Time {
	return a.dateOfBirth
}

func (a Authordata) DateOfDeath() time.Time {
	return a.dateOfDeath
}

func (a Authordata) Website() string {
	return a.website
}

func (a Authordata) Image() string {
	return a.image
}

func (a Authordata) InstanceOfHuman() bool {
	return a.instanceOf == isHuman
}

func (a Authordata) InstanceOfPseudonym() bool {
	return a.instanceOf == isPseudonym
}

func (a Authordata) InstanceOfPenName() bool {
	return a.instanceOf == isPenName
}

func (a Authordata) InstanceOfCollectivePseudonym() bool {
	return a.instanceOf == isCollectivePseudonym
}

func (a Authordata) Age() int {
	if a.dateOfBirth.IsZero() {
		return 0
	}
	if a.dateOfDeath.IsZero() {
		return time.Now().Year() - a.dateOfBirth.Year()
	}
	return a.dateOfDeath.Year() - a.dateOfBirth.Year()
}

func Author(name, language string) (Authordata, error) {
	var author Authordata
	id, err := getEntityId(name)
	if err != nil {
		return author, err
	}

	entitiesReq, err := gowikidata.NewGetEntities([]string{id})
	if err != nil {
		return author, err
	}
	entitiesReq.SetProps([]string{"descriptions", "claims"})
	entitiesReq.SetLanguages([]string{"en", language})
	// Call get to make the request based on the configurations
	entities, err := entitiesReq.Get()
	if err != nil {
		return author, err
	}

	if value, exists := (*entities)[id].Claims[claimInstanceOf]; exists {
		switch value[0].MainSnak.DataValue.Value.ValueFields.ID {
		case instanceOfHuman:
			author.instanceOf = isHuman
		case instanceOfPseudonym:
			author.instanceOf = isPseudonym
		case instanceOfPenName:
			author.instanceOf = isPenName
		case instanceOfCollectivePseudonym:
			author.instanceOf = isCollectivePseudonym
		default:
			return author, fmt.Errorf("instance of %s not supported", value[0].MainSnak.DataValue.Value.ValueFields.ID)
		}
	}

	author.wikidataEntityId = id
	author.retrievedOn = time.Now()

	author.description = (*entities)[id].Descriptions[language].Value
	if value, exists := (*entities)[id].Claims[claimDateOfBirth]; exists {
		author.dateOfBirth, err = time.Parse("2006-01-02T00:00:00Z", value[0].MainSnak.DataValue.Value.ValueFields.Time[1:])
		if err != nil {
			return author, err
		}
	}
	if value, exists := (*entities)[id].Claims[claimDateOfDeath]; exists {
		author.dateOfDeath, err = time.Parse("2006-01-02T00:00:00Z", value[0].MainSnak.DataValue.Value.ValueFields.Time[1:])
		if err != nil {
			return author, err
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
		author.image = fmt.Sprintf(imgUrl, img)
	}

	return author, nil
}

func getEntityId(name string) (string, error) {
	query, err := gowikidata.NewSearch(url.QueryEscape(name), "en")
	if err != nil {
		return "", err
	}
	result, err := query.Get()
	if err != nil {
		return "", err
	}
	return result.SearchResult[0].ID, nil
}
