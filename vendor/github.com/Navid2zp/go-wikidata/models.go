package gowikidata

import (
	"encoding/json"
	"strconv"
)

// WikiDataGetEntitiesRequest stores entities request url
type WikiDataGetEntitiesRequest struct {
	URL string
}

func (r *WikiDataGetEntitiesRequest) setParam(param string, values *[]string) {
	r.URL += createParam(param, *values)
}

// WikiDataGetClaimsRequest stores claims request url
type WikiDataGetClaimsRequest struct {
	URL string
}

func (r *WikiDataGetClaimsRequest) setParam(param string, values *[]string) {
	r.URL += createParam(param, *values)
}

// WikiDataSearchEntitiesRequest stores parameters for entities search
type WikiDataSearchEntitiesRequest struct {
	URL            string
	Limit          int
	Language       string
	Type           string
	Props          []string
	StrictLanguage bool
	Search         string
}

func (r *WikiDataSearchEntitiesRequest) setParam(param string, values *[]string) {
	r.URL += createParam(param, *values)
}

// Entity represents wikidata entities data
type Entity struct {
	ID           string                 `json:"id"`
	PageID       int                    `json:"pageid"`
	NS           int                    `json:"ns"`
	Title        string                 `json:"title"`
	LastRevID    int                    `json:"lastrevid"`
	Modified     string                 `json:"modified"`
	Type         string                 `json:"type"`
	Labels       map[string]Label       `json:"labels"`
	Descriptions map[string]Description `json:"descriptions"`
	Aliases      map[string][]Alias     `json:"aliases"`
	Claims       map[string][]Claim     `json:"claims"`
	SiteLinks    map[string]SiteLink    `json:"sitelinks"`
}

// GetDescription returns entity description in the given language code
func (e *Entity) GetDescription(languageCode string) string {
	return e.Descriptions[languageCode].Value
}

// GetLabel returns entity label in the given language code
func (e *Entity) GetLabel(languageCode string) string {
	return e.Labels[languageCode].Value
}

// Label represents wikidata labels data
type Label struct {
	Language    string `json:"language"`
	Value       string `json:"value"`
	ForLanguage string `json:"for-language"`
}

// Description represents wikidata descriptions data
type Description struct {
	Language    string `json:"language"`
	Value       string `json:"value"`
	ForLanguage string `json:"for-language"`
}

// Alias represents wikidata aliases data
type Alias struct {
	Language string `json:"language"`
	Value    string `json:"value"`
}

// SiteLink represents wikidata site links data
type SiteLink struct {
	Site   string   `json:"site"`
	Title  string   `json:"title"`
	Badges []string `json:"badges"`
	URL    string   `json:"url"`
}

// Claim represents wikidata claims data
type Claim struct {
	ID              string            `json:"id"`
	Rank            string            `json:"rank"`
	Type            string            `json:"type"`
	MainSnak        Snak              `json:"mainsnak"`
	Qualifiers      map[string][]Snak `json:"qualifiers"`
	QualifiersOrder []string          `json:"qualifiers-order"`
}

// Snak represents wikidata snak values
type Snak struct {
	SnakType  string    `json:"snaktype"`
	Property  string    `json:"property"`
	Hash      string    `json:"hash"`
	DataType  string    `json:"datatype"`
	DataValue DataValue `json:"datavalue"`
}

// DataValue represents wikidata values
// Wikidata values can be either string or number
// It will store the data type so you can work with it accordingly
type DataValue struct {
	Type  string           `json:"type"`
	Value DynamicDataValue `json:"value"`
}

// DynamicDataValue represents wikidata values for DataValue struct
type DynamicDataValue struct {
	Data        interface{}
	S           string
	I           int
	ValueFields DataValueFields
	Type        string
}

// UnmarshalJSON unmarshales given json result to DynamicDataValue
// It's main job is to find the data type and set the fields accordingly
func (d *DynamicDataValue) UnmarshalJSON(b []byte) (err error) {
	s := string(b)

	// If value starts with " and also ends with "
	// Then its string
	if string(s[0]) == "\"" && string(s[len(s)-1]) == "\"" {
		// Remove extra " from both sides of the string.
		cleaned := s[1 : len(s)-1]
		d.Data = cleaned
		d.S = cleaned
		d.Type = "String"
	} else {
		// If its int
		i, err := strconv.Atoi(s)
		if err != nil {
			// If its not int or string
			// Use DataValueFields
			values := DataValueFields{}
			err := json.Unmarshal(b, &values)
			if err != nil {
				return err
			}
			d.Type = "DataValueFields"
			d.ValueFields = values
			d.Data = values
		} else {
			// set value
			d.Type = "Int"
			d.I = i
			d.Data = i
		}
	}
	return
}

// DataValueFields represents wikidata value fields
type DataValueFields struct {
	EntityType    string  `json:"entity-type"`
	NumericID     int     `json:"numeric-id"`
	ID            string  `json:"id"`
	Type          string  `json:"type"`
	Value         string  `json:"value"`
	Time          string  `json:"time"`
	Precision     float64 `json:"precision"`
	Before        int     `json:"before"`
	After         int     `json:"after"`
	TimeZone      int     `json:"timezone"`
	CalendarModel string  `json:"calendarmodel"`
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	Globe         string  `json:"globe"`
	Amount        string  `json:"amount"`
	LowerBound    string  `json:"lowerbound"`
	UpperBound    string  `json:"upperbound"`
	Unit          string  `json:"unit"`
	Text          string  `json:"text"`
	Language      string  `json:"language"`
}

// Reference represents wikidata references
type Reference struct {
	Hash       string            `json:"hash"`
	Snaks      map[string][]Snak `json:"snaks"`
	SnaksOrder []string          `json:"snaks-order"`
}

// GetEntitiesResponse represents wikidata entities response
type GetEntitiesResponse struct {
	Entities map[string]Entity `json:"entities"`
	Success  uint              `json:"success"`
}

// GetClaimsResponse represents wikidata claims response
type GetClaimsResponse struct {
	Claims map[string][]Claim `json:"claims"`
}

// SearchEntity represents wikidata entities search
type SearchEntity struct {
	Repository  string      `json:"repository"`
	ID          string      `json:"id"`
	ConceptURI  string      `json:"concepturi"`
	Title       string      `json:"title"`
	PageID      int         `json:"pageid"`
	URL         string      `json:"url"`
	Label       string      `json:"label"`
	Description string      `json:"description"`
	Match       SearchMatch `json:"match"`
	DataType    string      `json:"datatype"`
}

// SearchMatch represents wikidata search match value
type SearchMatch struct {
	Type     string `json:"type"`
	Language string `json:"language"`
	Text     string `json:"text"`
}

// SearchInfo represents wikidata search info
type SearchInfo struct {
	Search string `json:"search"`
}

// SearchEntitiesResponse represents wikidata entities search response
type SearchEntitiesResponse struct {
	SearchInfo      SearchInfo     `json:"searchinfo"`
	SearchResult    []SearchEntity `json:"search"`
	SearchContinue  int            `json:"search-continue"`
	Success         uint           `json:"success"`
	CurrentContinue int
	SearchRequest   WikiDataSearchEntitiesRequest
}

// WikiPediaQuery represents wikipedia query
type WikiPediaQuery struct {
	BatchComplete string `json:"batchcomplete"`
	Query         struct {
		Pages map[string]struct {
			PageProps struct {
				WikiBaseItem string `json:"wikibase_item"`
			} `json:"pageprops"`
		} `json:"pages"`
	} `json:"query"`
}
