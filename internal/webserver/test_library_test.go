package webserver_test

import (
	"fmt"
	"html/template"
	"path/filepath"
	"time"

	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/precisiondate"
	"github.com/svera/coreander/v4/internal/webserver"
)

const testLibraryDir = "testdata/library"

func libraryCatalog() map[string]metadata.Metadata {
	pub := precisiondate.NewPrecisionDate("2020-01-01T00:00:00Z", precisiondate.PrecisionDay)
	catalog := map[string]metadata.Metadata{
		filepath.Join(testLibraryDir, "empty.epub"): {
			Title: "empty", Format: "EPUB", Language: "en",
		},
		filepath.Join(testLibraryDir, "empty.pdf"): {
			Title: "empty", Authors: []string{"Sergio Vera"}, Format: "PDF", Language: "en", Publication: pub,
		},
		filepath.Join(testLibraryDir, "metadata.epub"): {
			Title: "Test EPUB", Authors: []string{"John Doe"}, Format: "EPUB", Language: "en",
			Series: "The Lord of the Rings", Publication: pub, Subjects: []string{"Fiction"},
			Description: template.HTML("<p>Test</p>"),
		},
		filepath.Join(testLibraryDir, "metadata.pdf"): {
			Title: "Test PDF", Authors: []string{"John Doe"}, Format: "PDF", Language: "en", Publication: pub,
		},
		filepath.Join(testLibraryDir, "metadata_uppercase_ext.PDF"): {
			Title: "Test PDF", Authors: []string{"John Doe"}, Format: "PDF", Language: "en", Publication: pub,
		},
		filepath.Join(testLibraryDir, "nested/other.epub"): {
			Title: "Test EPUB", Authors: []string{"John Doe"}, Format: "EPUB", Language: "en", Publication: pub,
		},
		filepath.Join(testLibraryDir, "quijote.epub"): {
			Title: "Don Quijote de la Mancha", Authors: []string{"Miguel de Cervantes y Saavedra"},
			Format: "EPUB", Language: "es", Publication: pub,
		},
		filepath.Join(testLibraryDir, "quijote_another_edition.epub"): {
			Title: "Don Quijote de la Mancha", Authors: []string{"Miguel de Cervantes y Saavedra"},
			Format: "EPUB", Language: "es", Publication: pub,
		},
		filepath.Join(testLibraryDir, "quijote_third_edition.epub"): {
			Title: "Don Quijote de la Mancha", Authors: []string{"Miguel de Cervantes y Saavedra"},
			Format: "EPUB", Language: "es", Publication: pub,
		},
	}
	return catalog
}

type catalogReader struct {
	byPath map[string]metadata.Metadata
}

func (c catalogReader) Metadata(path string) (metadata.Metadata, error) {
	if m, ok := c.byPath[path]; ok {
		return m, nil
	}
	return metadata.Metadata{}, fmt.Errorf("no test metadata for %s", path)
}

func (c catalogReader) Cover(string, int) ([]byte, error) {
	return nil, nil
}

func testMetadataReaders() map[string]metadata.Reader {
	cr := catalogReader{byPath: libraryCatalog()}
	return map[string]metadata.Reader{
		".epub": cr,
		".pdf":  cr,
	}
}

func defaultTestConfig() webserver.Config {
	return webserver.Config{
		LibraryPath:           testLibraryDir,
		WordsPerMinute:        250,
		UploadDocumentMaxSize: 1,
		SessionTimeout:        24 * time.Hour,
		RecoveryTimeout:       2 * time.Hour,
	}
}
