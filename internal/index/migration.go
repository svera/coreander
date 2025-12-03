package index

import (
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v4/internal/precisiondate"
)

// MigrateAuthors migrates all authors from a legacy index to a new index.
// It searches only for items with Type = "author" in the legacy index.
func MigrateAuthors(oldIndex, newIndex bleve.Index) error {
	// Create a query that filters for authors only
	typeQuery := bleve.NewTermQuery(TypeAuthor)
	typeQuery.SetField("Type")

	matchAllQuery := bleve.NewMatchAllQuery()
	conjunctionQuery := bleve.NewConjunctionQuery()
	conjunctionQuery.AddQuery(matchAllQuery)
	conjunctionQuery.AddQuery(typeQuery)

	searchRequest := bleve.NewSearchRequest(conjunctionQuery)
	searchRequest.Size = 10000           // Process in batches
	searchRequest.Fields = []string{"*"} // Get all fields

	batch := newIndex.NewBatch()
	batchCount := 0

	for {
		searchResult, err := oldIndex.Search(searchRequest)
		if err != nil {
			return err
		}

		if searchResult.Total == 0 {
			break
		}

		// Migrate each author from search results
		for _, hit := range searchResult.Hits {
			// Convert search result to Author
			author := hydrateAuthorFromFields(hit.Fields, hit.ID)
			if author.Slug == "" {
				continue
			}

			// Ensure Type is set
			author.Type = TypeAuthor

			if err := batch.Index(author.Slug, author); err != nil {
				return err
			}
			batchCount++

			// Execute batch every 1000 items
			if batchCount >= 1000 {
				if err := newIndex.Batch(batch); err != nil {
					return err
				}
				batch = newIndex.NewBatch()
				batchCount = 0
			}
		}

		// If we got less than requested size, we're done
		if len(searchResult.Hits) < searchRequest.Size {
			break
		}

		// Move to next batch
		searchRequest.From += searchRequest.Size
	}

	// Execute any remaining authors
	if batchCount > 0 {
		if err := newIndex.Batch(batch); err != nil {
			return err
		}
	}

	return nil
}

// hydrateAuthorFromFields converts a fields map to an Author struct
func hydrateAuthorFromFields(fields map[string]interface{}, docID string) Author {
	retrievedOn := time.Time{}
	if val, ok := fields["RetrievedOn"]; ok && val != nil {
		if str, ok := val.(string); ok && str != "" {
			// Try RFC3339 format first (standard)
			if t, err := time.Parse(time.RFC3339, str); err == nil {
				retrievedOn = t
			} else if t, err := time.Parse("2006-01-02T15:04:05Z", str); err == nil {
				retrievedOn = t
			}
		}
	}

	dateOfBirth := precisiondate.PrecisionDate{Date: date.Zero}
	if val, ok := fields["DateOfBirth.Date"]; ok && val != nil {
		if dateVal, ok := val.(float64); ok {
			dateOfBirth.Date = date.Date(dateVal)
			if precVal, ok := fields["DateOfBirth.Precision"]; ok && precVal != nil {
				if prec, ok := precVal.(float64); ok {
					dateOfBirth.Precision = prec
				}
			}
		}
	}

	dateOfDeath := precisiondate.PrecisionDate{Date: date.Zero}
	if val, ok := fields["DateOfDeath.Date"]; ok && val != nil {
		if dateVal, ok := val.(float64); ok {
			dateOfDeath.Date = date.Date(dateVal)
			if precVal, ok := fields["DateOfDeath.Precision"]; ok && precVal != nil {
				if prec, ok := precVal.(float64); ok {
					dateOfDeath.Precision = prec
				}
			}
		}
	}

	name := ""
	if val, ok := fields["Name"]; ok && val != nil {
		if str, ok := val.(string); ok {
			name = str
		}
	}

	birthName := ""
	if val, ok := fields["BirthName"]; ok && val != nil {
		if str, ok := val.(string); ok {
			birthName = str
		}
	}

	slug := docID
	if val, ok := fields["Slug"]; ok && val != nil {
		if str, ok := val.(string); ok && str != "" {
			slug = str
		}
	}

	dataSourceID := ""
	if val, ok := fields["DataSourceID"]; ok && val != nil {
		if str, ok := val.(string); ok {
			dataSourceID = str
		}
	}

	website := ""
	if val, ok := fields["Website"]; ok && val != nil {
		if str, ok := val.(string); ok {
			website = str
		}
	}

	dataSourceImage := ""
	if val, ok := fields["DataSourceImage"]; ok && val != nil {
		if str, ok := val.(string); ok {
			dataSourceImage = str
		}
	}

	instanceOf := float64(0)
	if val, ok := fields["InstanceOf"]; ok && val != nil {
		if num, ok := val.(float64); ok {
			instanceOf = num
		}
	}

	gender := float64(0)
	if val, ok := fields["Gender"]; ok && val != nil {
		if num, ok := val.(float64); ok {
			gender = num
		}
	}

	author := Author{
		Name:            name,
		BirthName:       birthName,
		Slug:            slug,
		DataSourceID:    dataSourceID,
		RetrievedOn:     retrievedOn,
		WikipediaLink:   make(map[string]string),
		InstanceOf:      instanceOf,
		Description:     make(map[string]string),
		DateOfBirth:     dateOfBirth,
		DateOfDeath:     dateOfDeath,
		Website:         website,
		DataSourceImage: dataSourceImage,
		Gender:          gender,
		Pseudonyms:      slicer(fields["Pseudonyms"]),
	}

	// Extract Wikipedia links and descriptions for all languages
	for key, value := range fields {
		if strings.HasPrefix(key, "WikipediaLink.") {
			lang := strings.TrimPrefix(key, "WikipediaLink.")
			if str, ok := value.(string); ok {
				author.WikipediaLink[lang] = str
			}
		}
		if strings.HasPrefix(key, "Description.") {
			lang := strings.TrimPrefix(key, "Description.")
			if str, ok := value.(string); ok {
				author.Description[lang] = str
			}
		}
	}

	return author
}
