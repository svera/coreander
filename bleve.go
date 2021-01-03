package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/blevesearch/bleve"
	"github.com/blevesearch/bleve/analysis/lang/es"
	"github.com/pirmd/epub"
)

func create() (bleve.Index, error) {
	indexMapping := bleve.NewIndexMapping()
	esBookMapping := bleve.NewDocumentMapping()
	esBookMapping.DefaultAnalyzer = es.AnalyzerName
	languageFieldMapping := bleve.NewTextFieldMapping()
	languageFieldMapping.Index = false
	esBookMapping.AddFieldMappingsAt("language", languageFieldMapping)
	indexMapping.AddDocumentMapping("es", esBookMapping)
	index, err := bleve.New("coreander.db", indexMapping)
	if err != nil {
		return nil, err
	}
	return index, nil
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
		//fmt.Printf("%v", metadata.Creator[0].FullName)
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
		b := book{
			Title:       title,
			Author:      author,
			Description: description,
			Language:    language,
		}

		log.Printf("Indexing file %s\n", file)
		idx.Index(file, b)
	}
	return nil
}
