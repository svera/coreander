package index

import (
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/lang/de"
	"github.com/blevesearch/bleve/v2/analysis/lang/en"
	"github.com/blevesearch/bleve/v2/analysis/lang/es"
	"github.com/blevesearch/bleve/v2/analysis/lang/fr"
	"github.com/blevesearch/bleve/v2/analysis/lang/it"
	"github.com/blevesearch/bleve/v2/analysis/lang/pt"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/spf13/afero"
	"github.com/svera/coreander/internal/metadata"
)

var languages = []string{es.AnalyzerName, en.AnalyzerName, fr.AnalyzerName, de.AnalyzerName, it.AnalyzerName, pt.AnalyzerName}

type BleveIndexer struct {
	idx bleve.Index
}

func NewBleve(index bleve.Index) *BleveIndexer {
	return &BleveIndexer{index}
}

func CreateBleve(dir string) (*BleveIndexer, error) {
	indexMapping := bleve.NewIndexMapping()
	addLanguageMappings(indexMapping)
	index, err := bleve.New(dir+"/coreander/db", indexMapping)
	if err != nil {
		return nil, err
	}

	return &BleveIndexer{index}, nil
}

func CreateMemBleve() (*BleveIndexer, error) {
	indexMapping := bleve.NewIndexMapping()
	addLanguageMappings(indexMapping)
	index, err := bleve.NewMemOnly(indexMapping)
	if err != nil {
		return nil, err
	}

	return &BleveIndexer{index}, nil
}

func addLanguageMappings(indexMapping *mapping.IndexMappingImpl) {
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
func (b *BleveIndexer) Add(libraryPath string, fs afero.Fs, read map[string]metadata.Reader, batchSize int) error {
	libraryPath = strings.TrimSuffix(libraryPath, "/")
	batch := b.idx.NewBatch()
	e := afero.Walk(fs, libraryPath, func(path string, f os.FileInfo, err error) error {
		ext := filepath.Ext(path)
		if _, ok := read[ext]; !ok {
			return nil
		}
		meta, err := read[ext](path)
		if err != nil {
			log.Printf("Error extracting metadata from file %s: %s\n", path, err)
			return err
		}

		path = strings.Replace(path, libraryPath, "", 1)
		path = strings.TrimPrefix(path, "/")
		err = batch.Index(path, meta)
		if err != nil {
			log.Printf("Error indexing file %s: %s\n", path, err)
			return err
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

	queries := make([]query.Query, 0, len(languages))
	for i, lang := range languages {
		queries = append(queries, bleve.NewMatchQuery(keywords))
		queries[i].(*query.MatchQuery).Analyzer = lang
	}

	query := bleve.NewDisjunctionQuery(queries...)

	searchOptions := bleve.NewSearchRequestOptions(query, resultsPerPage, (page-1)*resultsPerPage, false)
	searchOptions.Fields = []string{"Title", "Author", "Description"}
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
		searchOptions.Fields = []string{"Title", "Author", "Description"}
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
		}
		result.Hits[val.ID] = doc
	}
	return &result, nil
}

// Count returns the number of indexed books
func (b *BleveIndexer) Count() (uint64, error) {
	return b.idx.DocCount()
}

func (b *BleveIndexer) Close() error {
	return b.idx.Close()
}

func calculateTotalPages(total, resultsPerPage uint64) int {
	return int(math.Ceil(float64(total) / float64(resultsPerPage)))
}
