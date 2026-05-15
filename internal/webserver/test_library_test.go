package webserver_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/precisiondate"
	"github.com/svera/coreander/v4/internal/webserver"
)

const testLibraryDir = "testdata/library"

// testLibraryRelPaths are document paths under testLibraryDir, sorted for stable slug suffixes.
var testLibraryRelPaths = []string{
	"empty.epub",
	"empty.pdf",
	"metadata.epub",
	"metadata.pdf",
	"metadata_uppercase_ext.PDF",
	"nested/other.epub",
	"quijote.epub",
	"quijote_another_edition.epub",
	"quijote_third_edition.epub",
}

func TestMain(m *testing.M) {
	if err := ensureTestdataOnDisk(); err != nil {
		log.Fatal(err)
	}
	os.Exit(m.Run())
}

func ensureTestdataOnDisk() error {
	if err := os.MkdirAll(testLibraryDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(testLibraryDir, "nested"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll("testdata/upload", 0o755); err != nil {
		return err
	}

	specs := []struct {
		rel     string
		title   string
		author  string
		series  string
		noTitle bool
	}{
		{"empty.epub", "", "", "", true},
		{"empty.pdf", "empty", "Sergio Vera", "", false},
		{"metadata.epub", "Test EPUB", "John Doe", "The Lord of the Rings", false},
		{"metadata.pdf", "Test PDF", "John Doe", "", false},
		{"metadata_uppercase_ext.PDF", "Test PDF", "John Doe", "", false},
		{"nested/other.epub", "Test EPUB", "John Doe", "", false},
		{"quijote.epub", "Don Quijote de la Mancha", "Miguel de Cervantes y Saavedra", "", false},
		{"quijote_another_edition.epub", "Don Quijote de la Mancha", "Miguel de Cervantes y Saavedra", "", false},
		{"quijote_third_edition.epub", "Don Quijote de la Mancha", "Miguel de Cervantes y Saavedra", "", false},
	}

	for _, spec := range specs {
		path := filepath.Join(testLibraryDir, spec.rel)
		if filepath.Ext(path) == ".pdf" {
			if err := writeStubPDF(path, spec.title, spec.author); err != nil {
				return err
			}
			continue
		}
		if err := writeMinimalEPUB(path, spec.title, spec.author, spec.series, spec.noTitle); err != nil {
			return err
		}
	}

	haruko := filepath.Join("testdata/upload", "haruko-html-jpeg.epub")
	if err := writeLargeEPUB(haruko, 2*1024*1024); err != nil {
		return err
	}
	childrens := filepath.Join("testdata/upload", "childrens-literature.epub")
	return writeMinimalEPUB(childrens, "Children's Literature", "Author", "", false)
}

func writeStubPDF(path, title, author string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	// Catalog reader supplies metadata; file content only needs to exist for FS operations.
	return os.WriteFile(path, []byte("%PDF-1.4\n%\n"), 0o644)
}

func writeMinimalEPUB(path, title, author, series string, noTitle bool) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	titleMeta := title
	if noTitle {
		titleMeta = ""
	}
	seriesXML := ""
	if series != "" {
		seriesXML = fmt.Sprintf(`<meta name="calibre:series" content="%s"/>`, series)
	}
	creatorXML := ""
	if author != "" {
		creatorXML = fmt.Sprintf(`<dc:creator opf:role="aut" file-as="%s">%s</dc:creator>`, author, author)
	}
	titleXML := ""
	if titleMeta != "" {
		titleXML = fmt.Sprintf(`<dc:title>%s</dc:title>`, titleMeta)
	}

	files := map[string]string{
		"mimetype":               "application/epub+zip",
		"META-INF/container.xml": `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`,
		"OEBPS/content.opf": fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    %s
    %s
    <dc:language>en</dc:language>
    %s
  </metadata>
  <manifest>
    <item id="ch1" href="chapter.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="ch1"/></spine>
</package>`, titleXML, creatorXML, seriesXML),
		"OEBPS/chapter.xhtml": `<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml"><body><p>Test content for word count.</p></body></html>`,
	}

	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(w, content); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func writeLargeEPUB(path string, size int) error {
	minBytes := int64(size) + 1
	if info, err := os.Stat(path); err == nil && info.Size() >= minBytes {
		return nil
	}
	padding := bytes.Repeat([]byte("x"), size)
	return writeMinimalEPUBWithExtra(path, padding)
}

func writeMinimalEPUBWithExtra(path string, extra []byte) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	header := &zip.FileHeader{
		Name:               "padding.bin",
		Method:             zip.Store,
		UncompressedSize64: uint64(len(extra)),
	}
	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	if _, err := w.Write(extra); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

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

func testLibraryFS() afero.Fs {
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll(filepath.Join(testLibraryDir, "nested"), 0o755)
	for _, rel := range testLibraryRelPaths {
		_ = afero.WriteFile(fs, filepath.Join(testLibraryDir, rel), []byte("stub"), 0o644)
	}
	return fs
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
