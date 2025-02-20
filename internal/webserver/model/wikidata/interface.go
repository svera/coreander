package wikidata

import gowikidata "github.com/Navid2zp/go-wikidata"

type wikidata interface {
	NewSearch(string, string) (searchEntitiesRequest, error)
	NewGetEntities([]string) (getEntitiesRequest, error)
}

type searchEntitiesRequest interface {
	Get() (*gowikidata.SearchEntitiesResponse, error)
}

type getEntitiesRequest interface {
	SetProps([]string)
	SetLanguages([]string)
	Get() (*map[string]gowikidata.Entity, error)
}
