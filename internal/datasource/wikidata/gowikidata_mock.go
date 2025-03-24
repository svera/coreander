package wikidata

import gowikidata "github.com/Navid2zp/go-wikidata"

type GowikidataMock struct {
	NewSearchFn      func(search string, language string) (SearchEntitiesRequest, error)
	NewGetEntitiesFn func(ids []string) (GetEntitiesRequest, error)
}

func (w GowikidataMock) NewSearch(search string, language string) (SearchEntitiesRequest, error) {
	return w.NewSearchFn(search, language)
}

func (w GowikidataMock) NewGetEntities(ids []string) (GetEntitiesRequest, error) {
	return w.NewGetEntitiesFn(ids)
}

type SearchEntitiesRequestMock struct {
	GetFn func() (*gowikidata.SearchEntitiesResponse, error)
}

func (e SearchEntitiesRequestMock) Get() (*gowikidata.SearchEntitiesResponse, error) {
	return e.GetFn()
}

type GetEntitiesRequestMock struct {
	SetPropsFn     func(props []string)
	SetLanguagesFn func(languages []string)
	GetFn          func() (*map[string]gowikidata.Entity, error)
}

func (e GetEntitiesRequestMock) SetProps(props []string) {
	e.SetPropsFn(props)
}
func (e GetEntitiesRequestMock) SetLanguages(languages []string) {
	e.SetLanguagesFn(languages)
}
func (e GetEntitiesRequestMock) Get() (*map[string]gowikidata.Entity, error) {
	return e.GetFn()
}
