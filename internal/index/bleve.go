package index

import (
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/blevesearch/bleve"
	"github.com/blevesearch/bleve/analysis/lang/es"
	"github.com/pirmd/epub"
)

type BleveIndexer struct {
	idx bleve.Index
}

func Open(dir string) (*BleveIndexer, error) {
	index, err := bleve.Open(dir + "/coreander/coreander.db")
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
	index, err := bleve.New(dir+"/coreander/coreander.db", indexMapping)
	if err != nil {
		return nil, err
	}
	if err != nil {
		log.Fatal(err)
	}

	return &BleveIndexer{index}, nil
}

func (b *BleveIndexer) Add(libraryPath string) error {
	// index some data
	fileList := make([]string, 0)
	e := filepath.Walk(libraryPath, func(path string, f os.FileInfo, err error) error {
		//e := filepath.Walk(".", func(path string, f os.FileInfo, err error) error {
		fileList = append(fileList, path)
		return err
	})

	if e != nil {
		return e
	}

	for _, file := range fileList {
		if filepath.Ext(file) != ".epub" {
			continue
		}
		metadata, err := epub.GetMetadataFromFile(file)
		if err != nil {
			log.Printf("Error indexing file %s: %s\n", file, err)
			continue
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
		bk := Book{
			Title:       title,
			Author:      author,
			Description: description,
			Language:    language,
		}

		file = strings.Replace(file, libraryPath, "", 1)
		b.idx.Index(file, bk)
	}
	return nil
}

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

func calculateTotalPages(total, resultsPerPage uint64) int {
	return int(math.Ceil(float64(total) / float64(resultsPerPage)))
}
