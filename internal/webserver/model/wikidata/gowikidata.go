package wikidata

import gowikidata "github.com/Navid2zp/go-wikidata"

type Gowikidata struct {
}

func (w Gowikidata) NewSearch(search string, language string) (searchEntitiesRequest, error) {
	return gowikidata.NewSearch(search, language)
}

func (w Gowikidata) NewGetEntities(ids []string) (getEntitiesRequest, error) {
	request, err := gowikidata.NewGetEntities(ids)

	return EntitiesRequest{request}, err
}

type EntitiesRequest struct {
	req *gowikidata.WikiDataGetEntitiesRequest
}

func (e EntitiesRequest) SetProps(props []string) {
	e.req.SetProps(props)
}
func (e EntitiesRequest) SetLanguages(languages []string) {
	e.req.SetLanguages(languages)
}
func (e EntitiesRequest) Get() (*map[string]gowikidata.Entity, error) {
	return e.req.Get()
}
