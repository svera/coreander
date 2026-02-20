package gowikidata

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

var (
	WikipediaDomain    = "https://en.wikipedia.org"
	WikidataDomain     = "https://www.wikidata.org"
	ImageResizerDomain = "https://commons.wikimedia.org"
)

const (
	wikipediaQueryURL = "%s/w/api.php?action=query&prop=pageprops&titles=%s&format=json"
	wikiDataAPIURL    = "%s/w/api.php?action=%s&format=json"
	imageResizerURL   = "%s/w/thumb.php?width=%d&f=%s"
)

// getHeaders returns a map of headers with proper User-Agent for API requests
func getHeaders() map[string]string {
	return map[string]string{
		"User-Agent": "Mozilla/5.0 (compatible; go-wikidata/1.0; +https://github.com/Navid2zp/go-wikidata)",
	}
}

// makeRequest makes an HTTP request with proper headers and returns the response
func makeRequest(method, url string, responseData interface{}) error {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return err
	}

	// Set headers
	headers := getHeaders()
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("request failed with status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return json.Unmarshal(body, responseData)
}

// GetPageItem returns Wikipedia page item ID
func GetPageItem(slug string) (string, error) {
	url := fmt.Sprintf(wikipediaQueryURL, WikipediaDomain, slug)

	wikipediaQuery := WikiPediaQuery{}
	err := makeRequest("GET", url, &wikipediaQuery)
	if err != nil {
		return "", err
	}

	item := ""
	// Get the first item
	for _, value := range wikipediaQuery.Query.Pages {
		item = value.PageProps.WikiBaseItem
		break
	}
	return item, nil
}

// NewGetEntities creates and returns a new WikiDataGetEntitiesRequest
// Use this function to initialize a request.
// It will return a pointer to WikiDataGetEntitiesRequest.
// You can make configurations on response.
func NewGetEntities(ids []string) (*WikiDataGetEntitiesRequest, error) {
	if len(ids) < 1 {
		return nil, errors.New("no ids provided")
	}

	req := WikiDataGetEntitiesRequest{
		URL: fmt.Sprintf(wikiDataAPIURL, WikidataDomain, "wbgetentities"),
	}
	req.setParam("ids", &ids)
	return &req, nil
}

// SetSites sets sites parameter for entities request
func (r *WikiDataGetEntitiesRequest) SetSites(sites []string) *WikiDataGetEntitiesRequest {
	r.setParam("sites", &sites)
	return r
}

// SetTitles sets titles parameter for entities request
func (r *WikiDataGetEntitiesRequest) SetTitles(titles []string) *WikiDataGetEntitiesRequest {
	r.setParam("titles", &titles)
	return r
}

// SetRedirects sets redirects parameter for entities request
func (r *WikiDataGetEntitiesRequest) SetRedirects(redirect bool) *WikiDataGetEntitiesRequest {
	redirectString := "yes"
	if !redirect {
		redirectString = "no"
	}
	r.URL += "&redirects=" + redirectString
	return r
}

// SetProps sets props parameter for entities request
// Default: info|sitelinks|aliases|labels|descriptions|claims|datatype
func (r *WikiDataGetEntitiesRequest) SetProps(props []string) *WikiDataGetEntitiesRequest {
	r.setParam("props", &props)
	return r
}

// SetLanguages sets languages parameter for entities request
func (r *WikiDataGetEntitiesRequest) SetLanguages(languages []string) *WikiDataGetEntitiesRequest {
	r.setParam("languages", &languages)
	return r
}

// SetLanguageFallback sets languagefallback parameter for entities request
func (r *WikiDataGetEntitiesRequest) SetLanguageFallback(fallback bool) *WikiDataGetEntitiesRequest {
	if fallback {
		r.URL += "&languagefallback="
	}
	return r
}

// SetNormalize sets normalize parameter for entities request
func (r *WikiDataGetEntitiesRequest) SetNormalize(normalize bool) *WikiDataGetEntitiesRequest {
	if normalize {
		r.URL += "&normalize="
	}
	return r
}

// SetSiteFilter sets sitefilter parameter for entities request
func (r *WikiDataGetEntitiesRequest) SetSiteFilter(sites []string) *WikiDataGetEntitiesRequest {
	r.setParam("sitefilter", &sites)
	return r
}

// Get makes a entities request and returns the response or an error
// Call this function after you finished configuring the request.
// It will send the request and unmarshales the response.
func (r *WikiDataGetEntitiesRequest) Get() (*map[string]Entity, error) {
	responseData := GetEntitiesResponse{}
	err := makeRequest("GET", r.URL, &responseData)
	if err != nil {
		return nil, err
	}
	return &responseData.Entities, nil
}

// NewGetClaims creates a new claim request for the given entity or claim
// WikiData action: wbgetclaims
// WikiData API page: https://www.wikidata.org/w/api.php?action=help&modules=wbgetclaims
// Either entity or claim must be provided
func NewGetClaims(entity, claim string) (*WikiDataGetClaimsRequest, error) {
	if entity == "" && claim == "" {
		return nil, errors.New("either entity or claim must be provided")
	}

	req := WikiDataGetClaimsRequest{
		URL: fmt.Sprintf(wikiDataAPIURL, WikidataDomain, "wbgetclaims"),
	}
	if entity != "" {
		req.setParam("entity", &[]string{entity})
	} else {
		req.setParam("claim", &[]string{claim})
	}
	return &req, nil
}

// NewGetClaims creates a new claim request for an entity
func (e *Entity) NewGetClaims() (*WikiDataGetClaimsRequest, error) {
	return NewGetClaims(e.ID, "")
}

// SetProperty sets property parameter for claims request
func (r *WikiDataGetClaimsRequest) SetProperty(property string) *WikiDataGetClaimsRequest {
	r.setParam("property", &[]string{property})
	return r
}

// SetRank sets rank parameter for claims request
// One of the following values: deprecated, normal, preferred
func (r *WikiDataGetClaimsRequest) SetRank(rank string) *WikiDataGetClaimsRequest {
	r.setParam("rank", &[]string{rank})
	return r
}

// SetProps sets props parameter for claims request
// Default: references
func (r *WikiDataGetClaimsRequest) SetProps(props []string) *WikiDataGetClaimsRequest {
	r.setParam("props", &props)
	return r
}

// Get creates a new request for claims and returns the response or an error
func (r *WikiDataGetClaimsRequest) Get() (*map[string][]Claim, error) {
	responseData := GetClaimsResponse{}
	err := makeRequest("GET", r.URL, &responseData)
	if err != nil {
		return nil, err
	}
	return &responseData.Claims, nil
}

// GetAvailableBadges returns a pointer to a list of strings.
// WikiData action: wbavailablebadges
// WikiData API page: https://www.wikidata.org/w/api.php?action=help&modules=wbavailablebadges
func GetAvailableBadges() ([]string, error) {
	var data struct{ Badges []string }
	url := fmt.Sprintf(wikiDataAPIURL, WikidataDomain, "wbavailablebadges")
	err := makeRequest("GET", url, &data)
	if err != nil {
		return nil, err
	}
	return data.Badges, nil
}

// NewSearch creates a new request for entities search and returns response or an error
// Create a request
// Both search and language are required
// WikiData action: wbsearchentities
// WikiData API page: https://www.wikidata.org/w/api.php?action=help&modules=wbsearchentities
func NewSearch(search, language string) (*WikiDataSearchEntitiesRequest, error) {
	req := WikiDataSearchEntitiesRequest{
		URL:      fmt.Sprintf(wikiDataAPIURL, WikidataDomain, "wbsearchentities"),
		Language: language,
		// default api value
		Limit: 7,
	}
	req.setParam("search", &[]string{search})
	req.setParam("language", &[]string{language})
	return &req, nil
}

// SetLimit sets limit parameter
// Default: 7
func (r *WikiDataSearchEntitiesRequest) SetLimit(limit int) *WikiDataSearchEntitiesRequest {
	r.Limit = limit
	r.setParam("limit", &[]string{strconv.Itoa(limit)})
	return r
}

// SetStrictLanguage sets strictlanguage parameter
func (r *WikiDataSearchEntitiesRequest) SetStrictLanguage(strictLanguage bool) *WikiDataSearchEntitiesRequest {
	if strictLanguage {
		r.URL += "&strictlanguage="
	}
	return r
}

// SetType sets type parameter
// One of the following values: item, property, lexeme, form, sense
// Default: item
func (r *WikiDataSearchEntitiesRequest) SetType(t string) *WikiDataSearchEntitiesRequest {
	r.setParam("type", &[]string{t})
	return r
}

// SetProps sets props parameter
// Default: url
func (r *WikiDataSearchEntitiesRequest) SetProps(props []string) *WikiDataSearchEntitiesRequest {
	r.setParam("props", &props)
	return r
}

// SetContinue sets continue parameter
// Default: 0
func (r *WikiDataSearchEntitiesRequest) SetContinue(c int) *WikiDataSearchEntitiesRequest {
	r.setParam("continue", &[]string{strconv.Itoa(c)})
	return r
}

// Get makes a entity search request and returns the response or an error
func (r *WikiDataSearchEntitiesRequest) Get() (*SearchEntitiesResponse, error) {
	responseData := SearchEntitiesResponse{}
	err := makeRequest("GET", r.URL, &responseData)
	if err != nil {
		return nil, err
	}

	responseData.SearchRequest = *r
	return &responseData, nil
}

// Next page of results for search
// Will use the same configurations as previous request
func (r *SearchEntitiesResponse) Next() (*SearchEntitiesResponse, error) {
	res, err := NewSearch(r.SearchRequest.Search, r.SearchRequest.Language)
	if err != nil {
		return nil, err
	}
	if r.SearchRequest.Limit > 0 {
		res.SetLimit(r.SearchRequest.Limit)
	}
	if r.SearchRequest.Type != "" {
		res.SetType(r.SearchRequest.Type)
	}
	if len(r.SearchRequest.Props) > 0 {
		res.SetProps(r.SearchRequest.Props)
	}
	if r.SearchRequest.StrictLanguage {
		res.SetStrictLanguage(true)
	}

	res.SetContinue(r.CurrentContinue + r.SearchRequest.Limit)
	response, err := res.Get()
	return response, err
}

// ImageResizer returns the url for resizing a wikimedia image to the given size
func ImageResizer(imageName string, size int) string {
	return fmt.Sprintf(imageResizerURL, ImageResizerDomain, size, imageName)
}

func createParam(param string, values []string) string {
	newString := "&" + param + "="
	valuesLen := len(values)
	for i, value := range values {
		newString += value
		if i+1 < valuesLen {
			newString += "|"
		}
	}
	return newString
}
