package wikidata

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/svera/coreander/v4/internal/precisiondate"
	"github.com/svera/coreander/v4/internal/webserver/controller/author"
)

const imgUrl = "https://upload.wikimedia.org/wikipedia/commons/%s/%s/%s"

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

	author.wikidataEntityId, author.instanceOf = getMostAccurateID(ids, entities)
	if author.instanceOf == InstanceUnknown {
		return author, nil
	}

	if value, exists := (*entities)[author.wikidataEntityId].Claims[propertyBirthName]; exists {
		author.birthName = value[0].MainSnak.DataValue.Value.ValueFields.Text
	} else if value, exists := (*entities)[author.wikidataEntityId].Claims[propertyNameInNativeLanguage]; exists {
		author.birthName = value[0].MainSnak.DataValue.Value.ValueFields.Text
	} else if value, exists := (*entities)[author.wikidataEntityId].Claims[propertyOfficialName]; exists {
		author.birthName = value[0].MainSnak.DataValue.Value.ValueFields.Text
	}

	if value, exists := (*entities)[author.wikidataEntityId].Claims[propertySexOrGender]; exists {
		author.gender = parseGender(value[0])
	}

	author.retrievedOn = time.Now().UTC()
	for _, lang := range languages {
		author.wikipediaLink[lang] = (*entities)[author.wikidataEntityId].SiteLinks[fmt.Sprintf("%swiki", lang)].URL
		author.description[lang] = (*entities)[author.wikidataEntityId].Descriptions[lang].Value
	}
	if claim, exists := (*entities)[author.wikidataEntityId].Claims[propertyDateOfBirth]; exists {
		author.dateOfBirth = parseDate(claim)
	}
	if claim, exists := (*entities)[author.wikidataEntityId].Claims[propertyDateOfDeath]; exists {
		author.dateOfDeath = parseDate(claim)
	}
	if value, exists := (*entities)[author.wikidataEntityId].Claims[propertyWebsite]; exists {
		author.website = value[0].MainSnak.DataValue.Value.S
	}
	if value, exists := (*entities)[author.wikidataEntityId].Claims[propertyPseudonym]; exists {
		author.pseudonyms = make([]string, 0, len(value))
		for _, claim := range value {
			pseudonym, err := strconv.Unquote("\"" + claim.MainSnak.DataValue.Value.S + "\"")
			if err != nil {
				continue
			}
			author.pseudonyms = append(author.pseudonyms, pseudonym)
		}
	}

	if value, exists := (*entities)[author.wikidataEntityId].Claims[propertyImage]; exists {
		img, err := strconv.Unquote("\"" + value[0].MainSnak.DataValue.Value.S + "\"")
		if err != nil {
			return nil, err
		}

		if slices.Contains([]string{".png", ".jpg", ".jpeg", ".tif", ".tiff"}, strings.ToLower(filepath.Ext(img))) {
			author.image = getImageUrl(filepath.Base(img))
		}
	}

	return author, nil
}

func getMostAccurateID(ids []string, entities *map[string]gowikidata.Entity) (string, float64) {
	for _, rank := range ranks {
		for _, id := range ids {
			claimValue, exists := (*entities)[id].Claims[propertyInstanceOf]
			if !exists {
				continue
			}
			if claimValue[0].Rank == rank {
				if instanceOf := parseInstanceOf(claimValue[0]); instanceOf != InstanceUnknown {
					return id, instanceOf
				}
			}
		}
	}

	return "", InstanceUnknown
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

func parseGender(claim gowikidata.Claim) float64 {
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

func parseInstanceOf(claim gowikidata.Claim) float64 {
	switch claim.MainSnak.DataValue.Value.ValueFields.ID {
	case qidInstanceOfHuman:
		return InstanceHuman
	case qidInstanceOfPseudonym:
		return InstancePseudonym
	case qidInstanceOfPenName:
		return InstancePenName
	case qidInstanceOfCollectivePseudonym:
		return InstanceCollectivePseudonym
	case qidInstanceOfHumanWhoseExistenceIsDisputed:
		return InstanceHumanWhoseExistenceIsDisputed
	}
	return InstanceUnknown
}

// parseDate parses a Wikidata time claim, returning a precisionDate.
// As there might be multiple dates for a single claim, we pick up the one ranked as preferred, if any.
// Otherwise, we return the first date.
func parseDate(claim []gowikidata.Claim) precisiondate.PrecisionDate {
	var date precisiondate.PrecisionDate

	for _, rank := range ranks {
		for _, v := range claim {
			if v.Rank == rank {
				return precisiondate.NewPrecisionDate(
					v.MainSnak.DataValue.Value.ValueFields.Time,
					v.MainSnak.DataValue.Value.ValueFields.Precision,
				)
			}
		}
	}

	return date
}

// getImageUrl will return a URL in the format
// https://upload.wikimedia.org/wikipedia/commons/a/ab/img_name.ext,
// where a and b are the first and the second chars of MD5 hashsum of the
// img_name.ext (with all whitespaces replaced by _)
func getImageUrl(filename string) string {
	u, err := url.QueryUnescape(filename)
	if err != nil {
		return ""
	}

	filename = strings.ReplaceAll(u, " ", "_")

	sum := md5.Sum([]byte(filename))
	hash := hex.EncodeToString(sum[:])

	return fmt.Sprintf(imgUrl, string(hash[0]), string(hash[0])+string(hash[1]), url.PathEscape(filename))
}
