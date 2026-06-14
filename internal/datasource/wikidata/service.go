package wikidata

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/precisiondate"
)

const imgUrl = "https://upload.wikimedia.org/wikipedia/commons/%s/%s/%s"

const maxEntitiesPerRequest = 50

var entityFetchProps = []string{"descriptions", "claims", "sitelinks/urls", "labels"}

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

func (a WikidataSource) SearchAuthor(name string, languages []string) (model.Author, error) {
	ids, err := a.SearchEntityIDs(name)
	if err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		return nil, nil
	}

	return a.RetrieveAuthor(ids, languages)
}

// SearchEntityIDs returns Wikidata entity IDs matching the given author name.
func (a WikidataSource) SearchEntityIDs(name string) ([]string, error) {
	return a.getEntityIds(name)
}

// RetrieveAuthor returns the first match from the list of passed Wikidata entity IDs that represents a human
func (a WikidataSource) RetrieveAuthor(ids []string, languages []string) (model.Author, error) {
	for _, id := range ids {
		if !validateID(id) {
			return Author{}, fmt.Errorf("invalid author ID %s", id)
		}
	}

	entities, err := a.fetchEntities(ids, languages)
	if err != nil {
		return nil, err
	}

	author, err := a.authorFromEntityIDs(ids, entities, languages)
	if err != nil {
		return nil, err
	}
	if author.instanceOf == InstanceUnknown {
		return nil, nil
	}
	return author, nil
}

// RetrieveAuthors fetches metadata for multiple authors in as few Wikidata requests as possible.
// Keys are caller-defined identifiers (for example author slugs). Values are Wikidata entity ID
// candidates for each author, in search-result order.
func (a WikidataSource) RetrieveAuthors(candidates map[string][]string, languages []string, batchInterval time.Duration) (map[string]model.Author, error) {
	if len(candidates) == 0 {
		return map[string]model.Author{}, nil
	}

	uniqueIDs := make([]string, 0)
	seen := make(map[string]struct{})
	for _, ids := range candidates {
		for _, id := range ids {
			if !validateID(id) {
				return nil, fmt.Errorf("invalid author ID %s", id)
			}
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			uniqueIDs = append(uniqueIDs, id)
		}
	}

	entities, err := a.fetchEntitiesBatched(uniqueIDs, languages, batchInterval)
	if err != nil {
		return nil, err
	}

	results := make(map[string]model.Author, len(candidates))
	for key, ids := range candidates {
		if len(ids) == 0 {
			continue
		}
		author, err := a.authorFromEntityIDs(ids, entities, languages)
		if err != nil {
			return nil, err
		}
		if author.instanceOf == InstanceUnknown {
			continue
		}
		results[key] = author
	}
	return results, nil
}

func (a WikidataSource) fetchEntities(ids []string, languages []string) (map[string]gowikidata.Entity, error) {
	return a.fetchEntitiesBatched(ids, languages, 0)
}

func (a WikidataSource) fetchEntitiesBatched(ids []string, languages []string, batchInterval time.Duration) (map[string]gowikidata.Entity, error) {
	entities := make(map[string]gowikidata.Entity, len(ids))
	if len(ids) == 0 {
		return entities, nil
	}

	for start := 0; start < len(ids); start += maxEntitiesPerRequest {
		if start > 0 && batchInterval > 0 {
			time.Sleep(batchInterval)
		}
		end := start + maxEntitiesPerRequest
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]

		entitiesReq, err := a.wikidata.NewGetEntities(chunk)
		if err != nil {
			return nil, err
		}
		entitiesReq.SetProps(entityFetchProps)
		entitiesReq.SetLanguages(languages)
		batch, err := entitiesReq.Get()
		if err != nil {
			return nil, err
		}
		for id, entity := range *batch {
			entities[id] = entity
		}
	}
	return entities, nil
}

func (a WikidataSource) authorFromEntityIDs(ids []string, entities map[string]gowikidata.Entity, languages []string) (Author, error) {
	author := Author{
		wikipediaLink: make(map[string]string),
		description:   make(map[string]string),
	}
	if len(ids) == 0 {
		return author, nil
	}

	entityPtr := &entities
	author.wikidataEntityId, author.instanceOf = getMostAccurateID(ids, entityPtr)
	if author.instanceOf == InstanceUnknown {
		return author, nil
	}

	entity, ok := entities[author.wikidataEntityId]
	if !ok {
		return author, nil
	}

	if value, exists := entity.Claims[propertyBirthName]; exists {
		author.birthName = value[0].MainSnak.DataValue.Value.ValueFields.Text
	} else if value, exists := entity.Claims[propertyNameInNativeLanguage]; exists {
		author.birthName = value[0].MainSnak.DataValue.Value.ValueFields.Text
	} else if value, exists := entity.Claims[propertyOfficialName]; exists {
		author.birthName = value[0].MainSnak.DataValue.Value.ValueFields.Text
	}

	if value, exists := entity.Claims[propertySexOrGender]; exists {
		author.gender = parseGender(value[0])
	}

	author.retrievedOn = time.Now().UTC()
	for _, lang := range languages {
		if url := entity.SiteLinks[fmt.Sprintf("%swiki", lang)].URL; url != "" {
			author.wikipediaLink[lang] = url
		}
		if description := entity.Descriptions[lang].Value; description != "" {
			author.description[lang] = description
		}
	}
	if claim, exists := entity.Claims[propertyDateOfBirth]; exists {
		author.dateOfBirth = parseDate(claim)
	}
	if claim, exists := entity.Claims[propertyDateOfDeath]; exists {
		author.dateOfDeath = parseDate(claim)
	}
	if value, exists := entity.Claims[propertyWebsite]; exists {
		author.website = value[0].MainSnak.DataValue.Value.S
	}
	if value, exists := entity.Claims[propertyPseudonym]; exists {
		author.pseudonyms = make([]string, 0, len(value))
		for _, claim := range value {
			pseudonym, err := strconv.Unquote("\"" + claim.MainSnak.DataValue.Value.S + "\"")
			if err != nil {
				continue
			}
			author.pseudonyms = append(author.pseudonyms, pseudonym)
		}
	}

	if value, exists := entity.Claims[propertyImage]; exists {
		img, err := strconv.Unquote("\"" + value[0].MainSnak.DataValue.Value.S + "\"")
		if err != nil {
			return Author{}, err
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
		return []string{}, nil
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

func validateID(id string) bool {
	if id == "" {
		return true
	}
	return regexp.MustCompile(`^[Qq]\d+$`).MatchString(id)
}
