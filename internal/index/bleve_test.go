package index_test

import (
	"reflect"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/metadata"
)

func TestIndexAndSearch(t *testing.T) {
	t.Skip("Incomplete test")
	idx, err := bleve.NewMemOnly(bleve.NewIndexMapping())

	if err != nil {
		t.Errorf("Error initialising index")
	}

	idx.Index("fileA", metadata.Metadata{
		Title:       "Test A",
		Author:      "Pérez",
		Description: "Just test metadata",
	})

	bv := index.NewBleve(idx)

	expected := index.Result{
		Page:       1,
		TotalPages: 1,
		TotalHits:  1,
		Hits: map[string]metadata.Metadata{
			"fileA": {
				Title:       "Test A",
				Author:      "Pérez",
				Description: "Just test metadata",
			},
		},
	}
	res, err := bv.Search("perez", 1, 10)

	if !reflect.DeepEqual(*res, expected) {
		t.Errorf("Wrong result returned, expected %v, got %v", expected, res)
	}
}

/*
func testCases() {
	var cases := []struct {
		filename       string
		meta           metadata.Metadata
		expectedResult index.Result
	}{
		{"fileA", http.StatusMovedPermanently},
		{"/es", http.StatusOK},
		{"/xx", http.StatusNotFound},
	}
}
*/
