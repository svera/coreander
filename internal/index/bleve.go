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
	idx         bleve.Index
	libraryPath string
	read        map[string]metadata.Reader
}

func NewBleve(index bleve.Index, libraryPath string, read map[string]metadata.Reader) *BleveIndexer {
	return &BleveIndexer{
		index,
		libraryPath,
		read,
	}
}

func AddLanguageMappings(indexMapping *mapping.IndexMappingImpl) {
	for _, lang := range languages {
		bookMapping := bleve.NewDocumentMapping()
		bookMapping.DefaultAnalyzer = lang
		languageFieldMapping := bleve.NewTextFieldMapping()
		languageFieldMapping.Index = false
		bookMapping.AddFieldMappingsAt("language", languageFieldMapping)
		indexMapping.AddDocumentMapping(lang, bookMapping)
	}
}

func (b *BleveIndexer) AddFile(file string) error {
	ext := filepath.Ext(file)
	if _, ok := b.read[ext]; !ok {
		return nil
	}
	meta, err := b.read[ext](file)
	if err != nil {
		log.Printf("Error extracting metadata from file %s: %s\n", file, err)
		return err
	}

	file = strings.Replace(file, b.libraryPath, "", 1)
	file = strings.TrimPrefix(file, "/")
	err = b.idx.Index(file, meta)
	if err != nil {
		log.Printf("Error indexing file %s: %s\n", file, err)
		return err
	}
	return nil
}

// AddLibrary scans <libraryPath> for books and adds them to the index in batches of <bathSize>
func (b *BleveIndexer) AddLibrary(fs afero.Fs, batchSize int) error {
	libraryPath := strings.TrimSuffix(b.libraryPath, "/")
	batch := b.idx.NewBatch()
	e := afero.Walk(fs, libraryPath, func(path string, f os.FileInfo, err error) error {
		ext := filepath.Ext(path)
		if _, ok := b.read[ext]; !ok {
			return nil
		}
		meta, err := b.read[ext](path)
		if err != nil {
			log.Printf("Error extracting metadata from file %s: %s\n", path, err)
			return nil
		}

		path = strings.Replace(path, libraryPath, "", 1)
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
