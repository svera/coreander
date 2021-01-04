package indexer

import (
	"log"
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

func New(dir, libraryPath string) (*BleveIndexer, error) {
	index, err := bleve.Open(dir + "/coreander/coreander.db")
	if err == bleve.ErrorIndexPathDoesNotExist {
		log.Println("No index found, creating a new one")
		indexMapping := bleve.NewIndexMapping()
		esBookMapping := bleve.NewDocumentMapping()
		esBookMapping.DefaultAnalyzer = es.AnalyzerName
		languageFieldMapping := bleve.NewTextFieldMapping()
		languageFieldMapping.Index = false
		esBookMapping.AddFieldMappingsAt("language", languageFieldMapping)
		indexMapping.AddDocumentMapping("es", esBookMapping)
		index, err = bleve.New(dir+"/coreander/coreander.db", indexMapping)
		if err != nil {
			return nil, err
		}
		if err != nil {
			log.Fatal(err)
		}
		err = add(index, libraryPath)
		if err != nil {
			log.Fatal(err)
		}
	}
	return &BleveIndexer{index}, nil
}

func add(idx bleve.Index, libraryPath string) error {
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
		bk := book{
			Title:       title,
			Author:      author,
			Description: description,
			Language:    language,
		}

		log.Printf("Indexing file %s\n", file)
		file = strings.Replace(file, libraryPath, "", 1)
		idx.Index(file, bk)
	}
	return nil
}

func (b *BleveIndexer) Search(keywords string, page, resultsPerPage int) (*bleve.SearchResult, error) {
	query := bleve.NewMatchQuery(keywords)
	search := bleve.NewSearchRequestOptions(query, resultsPerPage, (page-1)*resultsPerPage, false)
	search.Fields = []string{"Title", "Author", "Description"}
	searchResults, err := b.idx.Search(search)
	if err != nil {
		return nil, err
	}
	if searchResults.Total < uint64(page-1)*uint64(resultsPerPage) {
		page = 1
		search = bleve.NewSearchRequestOptions(query, resultsPerPage, (page-1)*resultsPerPage, false)
		search.Fields = []string{"Title", "Author", "Description"}
		searchResults, err = b.idx.Search(search)
		if err != nil {
			return nil, err
		}
	}
	return searchResults, nil
}
