package wikidata

import gowikidata "github.com/Navid2zp/go-wikidata"

type GowikidataMock struct {
	NewSearchFn      func(search string, language string) (searchEntitiesRequest, error)
	NewGetEntitiesFn func(ids []string) (getEntitiesRequest, error)
}

func (w GowikidataMock) NewSearch(search string, language string) (searchEntitiesRequest, error) {
	return w.NewSearchFn(search, language)
}

func (w GowikidataMock) NewGetEntities(ids []string) (getEntitiesRequest, error) {
	return w.NewGetEntitiesFn(ids)
}

type EntitiesRequestMock struct {
	SetPropsFn     func(props []string)
	SetLanguagesFn func(languages []string)
	GetFn          func() (*map[string]gowikidata.Entity, error)
}

func (e EntitiesRequestMock) SetProps(props []string) {
	e.SetPropsFn(props)
}
func (e EntitiesRequestMock) SetLanguages(languages []string) {
	e.SetLanguagesFn(languages)
}
func (e EntitiesRequestMock) Get() (*map[string]gowikidata.Entity, error) {
	return e.GetFn()
}
