package index

import (
	"fmt"
	"html/template"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blevesearch/bleve/analysis/token/lowercase"
	"github.com/blevesearch/bleve/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/char/asciifolding"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/spf13/afero"
	"github.com/svera/coreander/internal/metadata"
)

const wordsPerMinute = 300.0

type BleveIndexer struct {
	idx         bleve.Index
	libraryPath string
	reader      map[string]metadata.Reader
}

// NewBleve creates a new BleveIndexer instance using the passed parameters
func NewBleve(index bleve.Index, libraryPath string, read map[string]metadata.Reader) *BleveIndexer {
	return &BleveIndexer{
		index,
		strings.TrimSuffix(libraryPath, "/"),
		read,
	}
}

func AddMappings(indexMapping *mapping.IndexMappingImpl) {
	err := indexMapping.AddCustomAnalyzer("book",
		map[string]interface{}{
			"type": custom.Name,
			"char_filters": []string{
				asciifolding.Name,
			},
			"tokenizer": unicode.Name,
			"token_filters": []string{
				lowercase.Name,
			},
		})
	if err != nil {
		log.Fatal(err)
	}
	bookMapping := bleve.NewDocumentMapping()
	bookMapping.DefaultAnalyzer = "book"
	languageFieldMapping := bleve.NewTextFieldMapping()
	languageFieldMapping.Index = false
	bookMapping.AddFieldMappingsAt("language", languageFieldMapping)
	yearFieldMapping := bleve.NewTextFieldMapping()
	yearFieldMapping.Index = false
	bookMapping.AddFieldMappingsAt("year", yearFieldMapping)
	indexMapping.AddDocumentMapping("book", bookMapping)
}

// AddFile adds a file to the index
func (b *BleveIndexer) AddFile(file string) error {
	ext := filepath.Ext(file)
	if _, ok := b.reader[ext]; !ok {
		return nil
	}
	meta, err := b.reader[ext].Metadata(file)
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
		if _, ok := b.reader[ext]; !ok {
			return nil
		}
		meta, err := b.reader[ext].Metadata(path)
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

	query := bleve.NewQueryStringQuery(keywords)

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
			Description: template.HTML(val.Fields["Description"].(string)),
			Year:        val.Fields["Year"].(string),
			Words:       val.Fields["Words"].(float64),
		}
		if doc.Words != 0.0 {
			readingTime, err := time.ParseDuration(fmt.Sprintf("%fm", doc.Words/wordsPerMinute))
			if err == nil {
				doc.ReadingTime = fmtDuration(readingTime)
			}
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
