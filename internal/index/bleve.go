package index

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	//"github.com/blevesearch/bleve/v2/analysis/lang/de"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/simple"
	"github.com/blevesearch/bleve/v2/analysis/lang/en"

	// "github.com/blevesearch/bleve/v2/analysis/lang/es"
	// "github.com/blevesearch/bleve/v2/analysis/lang/fr"
	// "github.com/blevesearch/bleve/v2/analysis/lang/it"
	// "github.com/blevesearch/bleve/v2/analysis/lang/pt"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/spf13/afero"
	"github.com/svera/coreander/internal/metadata"
)

//var languages = []string{es.AnalyzerName, en.AnalyzerName, fr.AnalyzerName, de.AnalyzerName, it.AnalyzerName, pt.AnalyzerName}

type BleveIndexer struct {
	idx         bleve.Index
	libraryPath string
	read        map[string]metadata.Reader
}

// NewBleve creates a new BleveIndexer instance using the passed parameters
func NewBleve(index bleve.Index, libraryPath string, read map[string]metadata.Reader) *BleveIndexer {
	return &BleveIndexer{
		index,
		strings.TrimSuffix(libraryPath, "/"),
		read,
	}
}

func AddLanguageMappings(indexMapping *mapping.IndexMappingImpl) {
	//	for _, lang := range languages {
	bookMapping := bleve.NewDocumentMapping()
	bookMapping.DefaultAnalyzer = simple.Name
	languageFieldMapping := bleve.NewTextFieldMapping()
	languageFieldMapping.Index = false
	bookMapping.AddFieldMappingsAt("language", languageFieldMapping)
	yearFieldMapping := bleve.NewTextFieldMapping()
	yearFieldMapping.Index = false
	bookMapping.AddFieldMappingsAt("year", yearFieldMapping)
	wordsMapping := bleve.NewNumericFieldMapping()
	wordsMapping.Index = false
	bookMapping.AddFieldMappingsAt("words", wordsMapping)
	indexMapping.AddDocumentMapping(simple.Name, bookMapping)
	//	}
}

// AddFile adds a file to the index
func (b *BleveIndexer) AddFile(file string) error {
	ext := filepath.Ext(file)
	if _, ok := b.read[ext]; !ok {
		return nil
	}
	meta, err := b.read[ext](file)
	if err != nil {
		return fmt.Errorf("Error extracting metadata from file %s: %s", file, err)
	}

	file = strings.Replace(file, b.libraryPath, "", 1)
	file = strings.TrimPrefix(file, "/")
	err = b.idx.Index(file, meta)
	if err != nil {
		return fmt.Errorf("Error indexing file %s: %s", file, err)
	}
	return nil
}

// RemoveFile removes a file from the index
func (b *BleveIndexer) RemoveFile(file string) error {
	file = strings.Replace(file, b.libraryPath, "", 1)
	file = strings.TrimPrefix(file, "/")
	err := b.idx.Delete(file)
	if err != nil {
		return err
	}
	return nil
}

// AddLibrary scans <libraryPath> for books and adds them to the index in batches of <bathSize>
func (b *BleveIndexer) AddLibrary(fs afero.Fs, batchSize int) error {
	batch := b.idx.NewBatch()
	e := afero.Walk(fs, b.libraryPath, func(path string, f os.FileInfo, err error) error {
		ext := filepath.Ext(path)
		if _, ok := b.read[ext]; !ok {
			return nil
		}
		meta, err := b.read[ext](path)
		if err != nil {
			log.Printf("Error extracting metadata from file %s: %s\n", path, err)
			return nil
		}

		path = strings.Replace(path, b.libraryPath, "", 1)
		path = strings.TrimPrefix(path, "/")
		err = batch.Index(path, meta)
		if err != nil {
			log.Printf("Error indexing file %s: %s\n", path, err)
			return nil
		}

		if batch.Size() == batchSize {
			b.idx.Batch(batch)
			batch.Reset()
		}
		return nil
	})
	b.idx.Batch(batch)
	return e
}

// Search look for documents which match with the passed keywords. Returns a maximum <resultsPerPage> books, offset by <page>
func (b *BleveIndexer) Search(keywords string, page, resultsPerPage int) (*Result, error) {
	var result Result
	if page < 1 {
		page = 1
	}

	terms := strings.Split(keywords, " ")
	//queries := make([]query.Query, 0, len(languages))
	termQueries := make([]query.Query, 0, len(terms))
	//for _, lang := range languages {
	for j, term := range terms {
		termQueries = append(termQueries, bleve.NewMatchQuery(term))
		termQueries[j].(*query.MatchQuery).Analyzer = en.AnalyzerName
	}
	//queries = append(queries, bleve.NewConjunctionQuery(termQueries...))
	//}

	query := bleve.NewConjunctionQuery(termQueries...)

	searchOptions := bleve.NewSearchRequestOptions(query, resultsPerPage, (page-1)*resultsPerPage, false)
	searchOptions.Fields = []string{"Title", "Author", "Description", "Year", "Words"}
	searchResult, err := b.idx.Search(searchOptions)
	if err != nil {
		return nil, err
	}
	if searchResult.Total == 0 {
		return &result, nil
	}
	totalPages := calculateTotalPages(searchResult.Total, uint64(resultsPerPage))
	if totalPages < page {
		page = totalPages
		if page == 0 {
			page = 1
		}
		searchOptions = bleve.NewSearchRequestOptions(query, resultsPerPage, (page-1)*resultsPerPage, false)
		searchOptions.Fields = []string{"Title", "Author", "Description", "Year", "Words"}
		searchResult, err = b.idx.Search(searchOptions)
		if err != nil {
			return nil, err
		}
	}
	result = Result{
		Page:       page,
		TotalPages: totalPages,
		TotalHits:  int(searchResult.Total),
		Hits:       make(map[string]metadata.Metadata, len(searchResult.Hits)),
	}

	for _, val := range searchResult.Hits {
		doc := metadata.Metadata{
			Title:       val.Fields["Title"].(string),
			Author:      val.Fields["Author"].(string),
			Description: val.Fields["Description"].(string),
			Year:        val.Fields["Year"].(string),
			Words:       val.Fields["Words"].(float64),
		}
		readingTime, err := time.ParseDuration(fmt.Sprintf("%fm", doc.Words/300))
		if err == nil {
			doc.ReadingTime = fmtDuration(readingTime)
		}
		result.Hits[val.ID] = doc
	}
	return &result, nil
}

func fmtDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	return fmt.Sprintf("%dh %dm", h, m)
}

// Count returns the number of indexed books
func (b *BleveIndexer) Count() (uint64, error) {
	return b.idx.DocCount()
}

// Close closes the index
func (b *BleveIndexer) Close() error {
	return b.idx.Close()
}

func calculateTotalPages(total, resultsPerPage uint64) int {
	return int(math.Ceil(float64(total) / float64(resultsPerPage)))
}
