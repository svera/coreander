package index

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/blevesearch/bleve/analysis/lang/es"
	"github.com/pirmd/epub"
)

type BleveIndexer struct {
	idx bleve.Index
}

func Open(dir string) (*BleveIndexer, error) {
	index, err := bleve.Open(dir + "/coreander/db")
	if err != nil {
		return nil, err
	}
	return &BleveIndexer{index}, nil
}

func Create(dir string) (*BleveIndexer, error) {
	indexMapping := bleve.NewIndexMapping()
	esBookMapping := bleve.NewDocumentMapping()
	esBookMapping.DefaultAnalyzer = es.AnalyzerName
	languageFieldMapping := bleve.NewTextFieldMapping()
	languageFieldMapping.Index = false
	esBookMapping.AddFieldMappingsAt("language", languageFieldMapping)
	indexMapping.AddDocumentMapping("es", esBookMapping)
	index, err := bleve.New(dir+"/coreander/db", indexMapping)
	if err != nil {
		return nil, err
	}
	if err != nil {
		log.Fatal(err)
	}

	return &BleveIndexer{index}, nil
}

// Add scans <libraryPath> for books and adds them to the index in batches of <bathSize>
func (b *BleveIndexer) Add(libraryPath string, batchSize int) error {
	fileList, err := getFiles(libraryPath)
	if err != nil {
		return err
	}

	batch := b.idx.NewBatch()
	start := time.Now().Unix()
	for _, file := range fileList {
		if filepath.Ext(file) != ".epub" {
			continue
		}
		bk, err := getBookMetadata(file)
		if err != nil {
			log.Printf("Error extracting metadata from file %s: %s\n", file, err)
			continue
		}

		file = strings.Replace(file, libraryPath, "", 1)
		err = batch.Index(file, bk)
		if err != nil {
			log.Printf("Error indexing file %s: %s\n", file, err)
			continue
		}

		if batch.Size() == batchSize {
			b.idx.Batch(batch)
			batch.Reset()
		}
	}
	b.idx.Batch(batch)
	end := time.Now().Unix()
	dur, _ := time.ParseDuration(fmt.Sprintf("%ds", end-start))
	log.Println(fmt.Sprintf("Indexing finished, took %d seconds", int(dur.Seconds())))
	return nil
}

func getFiles(libraryPath string) ([]string, error) {
	fileList := make([]string, 0)
	e := filepath.Walk(libraryPath, func(path string, f os.FileInfo, err error) error {
		fileList = append(fileList, path)
		return err
	})

	if e != nil {
		return fileList, e
	}
	return fileList, nil
}

func getBookMetadata(file string) (Book, error) {
	bk := Book{}
	metadata, err := epub.GetMetadataFromFile(file)
	if err != nil {
		return bk, err
	}
	title := ""
	if len(metadata.Title) > 0 {
		title = metadata.Title[0]
	}
	author := ""
	if len(metadata.Creator) > 0 {
		author = metadata.Creator[0].FullName
	}
	description := ""
	if len(metadata.Description) > 0 {
		description = metadata.Description[0]
	}
	language := ""
	if len(metadata.Language) > 0 {
		language = metadata.Language[0]
	}
	bk = Book{
		Title:       title,
		Author:      author,
		Description: description,
		Language:    language,
	}
	return bk, nil
}

// Search look for books which match with the passed keywords. Returns a maximum <resultsPerPage> books, offset by <page>
func (b *BleveIndexer) Search(keywords string, page, resultsPerPage int) (*Results, error) {
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
	results := Results{
		Page:       page,
		TotalPages: totalPages,
		TotalHits:  int(searchResults.Total),
		Hits:       make(map[string]Book, len(searchResults.Hits)),
	}

	for _, val := range searchResults.Hits {
		bk := Book{
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
