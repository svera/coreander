# go-wikidata

[![GoDoc](https://godoc.org/github.com/Navid2zp/go-wikidata?status.svg)](https://pkg.go.dev/github.com/Navid2zp/go-wikidata?tab=doc)
[![Build](https://img.shields.io/github/workflow/status/Navid2zp/go-wikidata/Test)](https://github.com/Navid2zp/go-wikidata/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Navid2zp/go-wikidata)](https://goreportcard.com/report/github.com/Navid2zp/go-wikidata)
[![GitHub license](https://img.shields.io/github/license/Navid2zp/go-wikidata.svg)](https://github.com/Navid2zp/go-wikidata/blob/master/LICENSE)


Wikidata API bindings in golang.

This package is suitable for retrieving data from wikidata database using its public API.
Methods for updating, editing and adding to wikidata are not implemented (yet).

Read more about wikidata API: https://www.wikidata.org/w/api.php

## Contents

- [Installation](#install)
- [Get Entities](#get-entities)
    - [Methods](#get-entity-methods)
- [Get Claims](#get-claims)
    - [Methods](#get-claims-methods)
- [Search](#search)
    - [Methods](#search-methods)
- [Get Wikipedia Page Item](#get-wikipedia-page-item)
- [Get Available Badges](#get-available-badges)
- [License](#license)


### Install
```
go get github.com/Navid2zp/go-wikidata
```


### Get Entities

- Receives a list of entity ids.
- Response will be a pointer to `map[string]Entity` which the key being the entity ID and "Entity" being the data for that entity.
- WikiData action: `wbgetentities`
- WikiData API page: https://www.wikidata.org/w/api.php?action=help&modules=wbgetentities
```go
// Create a request
req, err := gowikidata.NewGetEntities([]string{"Q1"})

// Configurations such as props, sites and etc.
req.SetSites([]string{"enwiki", "fawiki"})

// Call get to make the request based on the configurations
res, err := req.Get()
```

###### Get entity methods:

Request methods:

```go
// Param: props
// Default: info|sitelinks|aliases|labels|descriptions|claims|datatype
req.SetProps([]string{"info", "claims"})

// Param: sites
req.SetSites([]string{"enwiki", "fawiki"})

// Param: sitefilter
req.SetSiteFilter([]string{"enwiki", "fawiki"})

// Param: normalize
req.SetNormalize(true)

// Param: languagefallback
req.SetLanguageFallback(true)

// Param: languages
req.SetLanguages([]string{"en", "fa"})

// Param: redirects
req.SetRedirects(true)

// Param: titles
req.SetTitles([]string{"title", "another"})


// Method chaining is also supported
req.SetNormalize(true).SetSites([]string{"enwiki", "fawiki"})
```

Response methods:
```go
claimReq, err := res["Q1"].NewGetClaims()
```

Same as calling `NewGetClaims`. See "Get Claims" for more information.


### Get Claims

- Receives an entity ID or a claim GUID.
- Response will be a pointer to `map[string][]Claim` which the key being the entity ID and value being a list of claims for that entity.
- WikiData action: `wbgetclaims`
- WikiData API page: https://www.wikidata.org/w/api.php?action=help&modules=wbgetclaims

```go
// Create a request
// You must either provide entity id or a claim GUID
req, err := gowikidata.NewGetClaims("Q1", "")

// Call get to make the request based on the configurations
res, err := req.Get()
```

You can also call `NewGetClaims` on `Entity` type.

###### Get claims methods:

Request methods:

```go
// Param: props
// Default: references
req.SetProps([]string{"references"})

// Param: rank
// One of the following values: deprecated, normal, preferred
req.SetRank("normal")

// Param: property
req.SetProperty("P31")
```

### Search

- Receives search string and search language string.
- Response will be a pointer to `SearchEntitiesResponse` type containing the result as `SearchEntitiesResponse.SearchResult`.
- WikiData action: `wbsearchentities`
- WikiData API page: https://www.wikidata.org/w/api.php?action=help&modules=wbsearchentities

```go
// Create a request
// Both search and language are required
req, err := gowikidata.NewSearch("abc", "en")

// Call get to make the request based on the configurations
res, err := req.Get()
```

###### Search methods:

Request methods:

```go
// Param: props
// Default: url
req.SetProps([]string{"url"})

// Param: limit
// Default: 7
req.SetLimit(10)

// Param: strictlanguage
req.SetStrictLanguage(true)

// Param: type
// One of the following values: item, property, lexeme, form, sense
// Default: item
req.SetType("item")

// Param: continue
// Default: 0
req.SetContinue(7)
```

Response methods:
```go
// Next page of results
// new coninue = limit + previous continue value
nextPage, err := res.Next()
```

### Get Wikipedia Page Item

Find Wikipedia page item ID in wikidata by page slug (https://en.wikipedia.org/wiki/[SLUG]).

```go
wikiDataID, err := gowikidata.GetPageItem("Earth")
fmt.Println(wikiDataID) // "Q2"
```



### Get Available Badges

- Returns a pointer to a list of strings.
- WikiData action: `wbavailablebadges`
- WikiData API page: https://www.wikidata.org/w/api.php?action=help&modules=wbavailablebadges

```go
badges, err := gowikidata.GetAvailableBadges()
```

License
----

MIT
