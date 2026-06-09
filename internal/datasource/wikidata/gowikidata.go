package wikidata

import gowikidata "github.com/Navid2zp/go-wikidata"

type Gowikidata struct {
}

func (w Gowikidata) NewSearch(search string, language string) (SearchEntitiesRequest, error) {
	req, err := gowikidata.NewSearch(search, language)
	if err != nil {
		return nil, err
	}
	return searchRequest{url: req.URL}, nil
}

func (w Gowikidata) NewGetEntities(ids []string) (GetEntitiesRequest, error) {
	request, err := gowikidata.NewGetEntities(ids)
	if err != nil {
		return nil, err
	}
	return entitiesRequest{req: request}, nil
}

type searchRequest struct {
	url string
}

func (s searchRequest) Get() (*gowikidata.SearchEntitiesResponse, error) {
	var response gowikidata.SearchEntitiesResponse
	if err := apiHTTPClient.getJSON(s.url, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

type entitiesRequest struct {
	req *gowikidata.WikiDataGetEntitiesRequest
}

func (e entitiesRequest) SetProps(props []string) {
	e.req.SetProps(props)
}
func (e entitiesRequest) SetLanguages(languages []string) {
	e.req.SetLanguages(languages)
}
func (e entitiesRequest) Get() (*map[string]gowikidata.Entity, error) {
	var response gowikidata.GetEntitiesResponse
	if err := apiHTTPClient.getJSON(e.req.URL, &response); err != nil {
		return nil, err
	}
	return &response.Entities, nil
}
