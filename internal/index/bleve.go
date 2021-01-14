package index

import (
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/blevesearch/bleve"
	"github.com/blevesearch/bleve/analysis/lang/de"
	"github.com/blevesearch/bleve/analysis/lang/en"
	"github.com/blevesearch/bleve/analysis/lang/es"
	"github.com/blevesearch/bleve/analysis/lang/fr"
	"github.com/blevesearch/bleve/analysis/lang/it"
	"github.com/blevesearch/bleve/analysis/lang/pt"
	"github.com/blevesearch/bleve/mapping"
	"github.com/svera/coreander/metadata"
)

type BleveIndexer struct {
	idx bleve.Index
}

func NewBleve(index bleve.Index) *BleveIndexer {
	return &BleveIndexer{index}
}

func CreateBleve(dir string) (*BleveIndexer, error) {
	indexMapping := bleve.NewIndexMapping()
	addLanguageMappings(indexMapping, []string{es.AnalyzerName, en.AnalyzerName, fr.AnalyzerName, de.AnalyzerName, it.AnalyzerName, pt.AnalyzerName})
	index, err := bleve.New(dir+"/coreander/db", indexMapping)
	if err != nil {
		return nil, err
	}

	return &BleveIndexer{index}, nil
}

func addLanguageMappings(indexMapping *mapping.IndexMappingImpl, languages []string) {
	for _, lang := range languages {
		bookMapping := bleve.NewDocumentMapping()
		bookMapping.DefaultAnalyzer = lang
		languageFieldMapping := bleve.NewTextFieldMapping()
		languageFieldMapping.Index = false
		bookMapping.AddFieldMappingsAt("language", languageFieldMapping)
		indexMapping.AddDocumentMapping(lang, bookMapping)
	}
}

// Add scans <libraryPath> for books and adds them to the index in batches of <bathSize>
func (b *BleveIndexer) Add(libraryPath string, read map[string]metadata.Reader, batchSize int) error {
	batch := b.idx.NewBatch()
	e := filepath.Walk(libraryPath, func(path string, f os.FileInfo, err error) error {
		ext := filepath.Ext(path)
		if _, ok := read[ext]; !ok {
			return nil
		}
		meta, err := read[ext](path)
		if err != nil {
			log.Printf("Error extracting metadata from file %s: %s\n", path, err)
			return nil
		}

		path = strings.Replace(path, libraryPath, "", 1)
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

// Search look for books which match with the passed keywords. Returns a maximum <resultsPerPage> books, offset by <page>
func (b *BleveIndexer) Search(keywords string, page, resultsPerPage int) (*Result, error) {
	if page < 1 {
		page = 1
	}
	query := bleve.NewMatchQuery(keywords)
	searchOptions := bleve.NewSearchRequestOptions(query, resultsPerPage, (page-1)*resultsPerPage, false)
	searchOptions.Fields = []string{"Title", "Author", "Description"}
	searchResults, err := b.idx.Search(searchOptions)
	if err != nil {
		return nil, err
	}
	totalPages := calculateTotalPages(searchResults.Total, uint64(resultsPerPage))
	if totalPages < page {
		page = totalPages
		if page == 0 {
			page = 1
		}
		searchOptions = bleve.NewSearchRequestOptions(query, resultsPerPage, (page-1)*resultsPerPage, false)
		searchOptions.Fields = []string{"Title", "Author", "Description"}
		searchResults, err = b.idx.Search(searchOptions)
		if err != nil {
			return nil, err
		}
	}
	results := Result{
		Page:       page,
		TotalPages: totalPages,
		TotalHits:  int(searchResults.Total),
		Hits:       make(map[string]metadata.Metadata, len(searchResults.Hits)),
	}

	for _, val := range searchResults.Hits {
		bk := metadata.Metadata{
			Title:       val.Fields["Title"].(string),
			Author:      val.Fields["Author"].(string),
			Description: val.Fields["Description"].(string),
		}
		results.Hits[val.ID] = bk
	}
	return &results, nil
}

// Count returns the number of indexed books
func (b *BleveIndexer) Count() (uint64, error) {
	return b.idx.DocCount()
}

func calculateTotalPages(total, resultsPerPage uint64) int {
	return int(math.Ceil(float64(total) / float64(resultsPerPage)))
}
