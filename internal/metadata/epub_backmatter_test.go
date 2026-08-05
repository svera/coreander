package metadata

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pirmd/epub"
)

// writeTestEpub builds a minimal EPUB zip file from files (path -> content)
// in a temp directory and returns its path.
func writeTestEpub(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	epubPath := filepath.Join(dir, "test.epub")
	f, err := os.Create(epubPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	return epubPath
}

const testContainerXML = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
    <rootfiles>
        <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
    </rootfiles>
</container>`

func TestTextFromZipExcludesEPUB2GuideBibliography(t *testing.T) {
	opfXML := `<?xml version="1.0" encoding="utf-8"?>
<package version="2.0" unique-identifier="BookId" xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Test Book</dc:title>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>
    <item id="biblio" href="bibliography.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="content"/>
    <itemref idref="biblio"/>
  </spine>
  <guide>
    <reference type="bibliography" href="bibliography.xhtml" title="Bibliography"/>
  </guide>
</package>`

	epubPath := writeTestEpub(t, map[string]string{
		"META-INF/container.xml":   testContainerXML,
		"OEBPS/content.opf":        opfXML,
		"OEBPS/content.xhtml":      `<html><body><p>narrativemarker actual story text</p></body></html>`,
		"OEBPS/bibliography.xhtml": `<html><body><p>citationmarker Stevenson, War, London: repeated citation</p></body></html>`,
	})

	book, err := epub.Open(epubPath)
	if err != nil {
		t.Fatalf("epub.Open returned an error: %s", err)
	}
	defer book.Close()

	opf, err := book.Package()
	if err != nil {
		t.Fatalf("book.Package returned an error: %s", err)
	}

	text, err := textFromZip(book.ReadCloser, opf)
	if err != nil {
		t.Fatalf("textFromZip returned an error: %s", err)
	}

	if !strings.Contains(text, "narrativemarker") {
		t.Errorf("expected extracted text to contain the content chapter, got %q", text)
	}
	if strings.Contains(text, "citationmarker") {
		t.Errorf("expected extracted text to exclude the guide-referenced bibliography chapter, got %q", text)
	}
}

func TestTextFromZipExcludesEPUB3LandmarksNotes(t *testing.T) {
	opfXML := `<?xml version="1.0" encoding="utf-8"?>
<package version="3.0" unique-identifier="BookId" xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Test Book</dc:title>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>
    <item id="notes" href="notes.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="content"/>
    <itemref idref="notes"/>
  </spine>
</package>`

	navXML := `<html xmlns:epub="http://www.idpf.org/2007/ops"><body>
  <nav epub:type="toc"><ol><li><a href="content.xhtml">Content</a></li></ol></nav>
  <nav epub:type="landmarks"><ol>
    <li><a epub:type="bodymatter" href="content.xhtml">Start</a></li>
    <li><a epub:type="notes" href="notes.xhtml">Notes</a></li>
  </ol></nav>
</body></html>`

	epubPath := writeTestEpub(t, map[string]string{
		"META-INF/container.xml": testContainerXML,
		"OEBPS/content.opf":      opfXML,
		"OEBPS/nav.xhtml":        navXML,
		"OEBPS/content.xhtml":    `<html><body><p>narrativemarker actual story text</p></body></html>`,
		"OEBPS/notes.xhtml":      `<html><body><p>citationmarker Stevenson, War, London: repeated citation</p></body></html>`,
	})

	book, err := epub.Open(epubPath)
	if err != nil {
		t.Fatalf("epub.Open returned an error: %s", err)
	}
	defer book.Close()

	opf, err := book.Package()
	if err != nil {
		t.Fatalf("book.Package returned an error: %s", err)
	}

	text, err := textFromZip(book.ReadCloser, opf)
	if err != nil {
		t.Fatalf("textFromZip returned an error: %s", err)
	}

	if !strings.Contains(text, "narrativemarker") {
		t.Errorf("expected extracted text to contain the content chapter, got %q", text)
	}
	if strings.Contains(text, "citationmarker") {
		t.Errorf("expected extracted text to exclude the landmarks-referenced notes chapter, got %q", text)
	}
}

func TestTextFromZipKeepsAllContentWhenNoBackmatterIsDeclared(t *testing.T) {
	opfXML := `<?xml version="1.0" encoding="utf-8"?>
<package version="2.0" unique-identifier="BookId" xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Test Book</dc:title>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="content"/>
  </spine>
</package>`

	epubPath := writeTestEpub(t, map[string]string{
		"META-INF/container.xml": testContainerXML,
		"OEBPS/content.opf":      opfXML,
		"OEBPS/content.xhtml":    `<html><body><p>narrativemarker actual story text</p></body></html>`,
	})

	book, err := epub.Open(epubPath)
	if err != nil {
		t.Fatalf("epub.Open returned an error: %s", err)
	}
	defer book.Close()

	opf, err := book.Package()
	if err != nil {
		t.Fatalf("book.Package returned an error: %s", err)
	}

	text, err := textFromZip(book.ReadCloser, opf)
	if err != nil {
		t.Fatalf("textFromZip returned an error: %s", err)
	}

	if !strings.Contains(text, "narrativemarker") {
		t.Errorf("expected extracted text to contain the content chapter, got %q", text)
	}
}

// TestTextFromZipFallsBackToFilenamePatterns guards against a real case: a
// fan/community-converted EPUB (common for older or shared Spanish-language
// books) whose OPF <guide> only bothers declaring the cover, and has no
// EPUB3 nav landmarks either, even though its spine plainly contains files
// named "Notas1.xhtml", "Notas2.xhtml", "Bibliografia.xhtml", etc. Since
// backmatterHrefs has no guide/landmarks references to go on, it falls back
// to matching common bibliography/notes filename patterns directly.
func TestTextFromZipFallsBackToFilenamePatterns(t *testing.T) {
	opfXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>
<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="BookId" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Test Book</dc:title>
    <dc:language>es</dc:language>
  </metadata>
  <manifest>
    <item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>
    <item id="notas1" href="Notas1.xhtml" media-type="application/xhtml+xml"/>
    <item id="biblio" href="Bibliografia.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="content"/>
    <itemref idref="notas1"/>
    <itemref idref="biblio"/>
  </spine>
  <guide>
    <reference href="content.xhtml" title="Cover" type="cover"/>
  </guide>
</package>`

	epubPath := writeTestEpub(t, map[string]string{
		"META-INF/container.xml":   testContainerXML,
		"OEBPS/content.opf":        opfXML,
		"OEBPS/content.xhtml":      `<html><body><p>narrativemarker actual story text</p></body></html>`,
		"OEBPS/Notas1.xhtml":       `<html><body><p>citationmarker Stevenson, War, London: repeated citation</p></body></html>`,
		"OEBPS/Bibliografia.xhtml": `<html><body><p>bibliomarker Another Author, Another Book, City: publisher</p></body></html>`,
	})

	book, err := epub.Open(epubPath)
	if err != nil {
		t.Fatalf("epub.Open returned an error: %s", err)
	}
	defer book.Close()

	opf, err := book.Package()
	if err != nil {
		t.Fatalf("book.Package returned an error: %s", err)
	}

	text, err := textFromZip(book.ReadCloser, opf)
	if err != nil {
		t.Fatalf("textFromZip returned an error: %s", err)
	}

	if !strings.Contains(text, "narrativemarker") {
		t.Errorf("expected extracted text to contain the content chapter, got %q", text)
	}
	if strings.Contains(text, "citationmarker") {
		t.Errorf("expected extracted text to exclude Notas1.xhtml via the filename fallback, got %q", text)
	}
	if strings.Contains(text, "bibliomarker") {
		t.Errorf("expected extracted text to exclude Bibliografia.xhtml via the filename fallback, got %q", text)
	}
}
