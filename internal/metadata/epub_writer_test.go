package metadata

import (
	"encoding/xml"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/pirmd/epub"
)

func TestWriteAuthorIDs(t *testing.T) {
	for _, version := range []string{"2.0", "3.0"} {
		t.Run(version, func(t *testing.T) {
			filename := writeTestEpub(t, map[string]string{
				"META-INF/container.xml": testContainerXML,
				"OEBPS/content.opf": fmt.Sprintf(`<package xmlns="http://www.idpf.org/2007/opf" version="%s" unique-identifier="book"%s>
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
<dc:identifier id="book">test-id</dc:identifier>
<dc:title>Test book</dc:title><dc:language>en</dc:language>
<dc:creator id="douglas" opf:role="aut" opf:file-as="Adams, Douglas">Douglas Adams</dc:creator>
<dc:creator id="other">Other Author</dc:creator>
<dc:creator id="shared">Author One &amp; Author Two</dc:creator>
<dc:contributor id="artist" opf:role="ill">Artist</dc:contributor>
<dc:contributor id="translator" opf:role="trl">Douglas Adams</dc:contributor>
<meta name="custom" content="keep"/><meta name="calibre:series" content="Series"/>
</metadata><manifest><item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/></manifest>
<spine><itemref idref="chapter"/></spine></package>`, version, testWriterPrefix(version)),
				"OEBPS/chapter.xhtml": "<html><body>Unchanged chapter.</body></html>",
			})
			reader := NewEpubReader()
			info, err := epub.GetMetadataFromFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			info.Creator[0].Attributes = []xml.Attr{{Name: xml.Name{Local: "custom"}, Value: "Keep this"}}
			if version == "3.0" {
				info.Creator[0].Refinements = []epub.Meta{{Property: "alternate-script", Value: "Keep this"}}
			}
			if err := epub.WriteMetadataToFile(filename, &epub.Information{Creator: info.Creator}); err != nil {
				t.Fatal(err)
			}

			ids := map[string]string{
				"douglas-adams": "Q42", "artist": "Q123",
				"author-one": "Q1", "author-two": "Q2",
			}
			changed, err := reader.WriteAuthorIDs(filename, ids)
			if err != nil || !changed {
				t.Fatalf("changed=%v, err=%v", changed, err)
			}
			info, err = epub.GetMetadataFromFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			assertAuthorID(t, info.Creator[0], "Q42", version)
			assertAuthorID(t, info.Creator[1], "", version)
			assertAuthorID(t, info.Creator[2], "Q1 Q2", version)
			assertAuthorID(t, info.Contributor[0], "Q123", version)
			assertAuthorID(t, info.Contributor[1], "", version)
			if info.Creator[0].FileAs != "Adams, Douglas" || info.Creator[0].Role != "aut" {
				t.Fatalf("lost author attributes: %+v", info.Creator[0])
			}
			preserved := false
			for _, a := range info.Creator[0].Attributes {
				preserved = preserved || (a.Name == (xml.Name{Local: "custom"}) && a.Value == "Keep this")
			}
			refinementPreserved := version != "3.0"
			for _, m := range info.Creator[0].Refinements {
				refinementPreserved = refinementPreserved || (m.Property == "alternate-script" && m.Value == "Keep this")
			}
			if !preserved || !refinementPreserved || info.Title[0].String() != "Test book" || info.Series.String() != "Series" {
				t.Fatal("unrelated metadata was not preserved")
			}
			if version == "3.0" && (info.Prefixes["cdr"] != CoreanderMetadataPrefix || info.Prefixes["other"] != "https://example.org/other/") {
				t.Fatalf("unexpected prefixes: %v", info.Prefixes)
			}
			book, err := epub.Open(filename)
			if err != nil {
				t.Fatal(err)
			}
			chapter, err := readZipFile(book.ReadCloser, "OEBPS/chapter.xhtml")
			opf, opfErr := readZipFile(book.ReadCloser, "OEBPS/content.opf")
			book.Close()
			if err != nil || string(chapter) != "<html><body>Unchanged chapter.</body></html>" {
				t.Fatalf("chapter changed: %s, %v", chapter, err)
			}
			if opfErr != nil || strings.Contains(string(opf), "epub:refinement") {
				t.Fatalf("unexpected legacy refinement wrapper in package: %v", opfErr)
			}
			if version == "2.0" && (!strings.Contains(string(opf), `wd_id="Q42"`) || !strings.Contains(string(opf), CoreanderMetadataPrefix)) {
				t.Fatal("EPUB 2 author ID was not written as a namespaced direct attribute")
			}
			if version == "2.0" {
				output := string(opf)
				if !strings.Contains(output, `cdr:wd_id="Q42"`) || strings.Count(output, `xmlns:cdr=`) != 1 {
					t.Fatal("EPUB 2 must use the preferred cdr prefix declared once")
				}
				for _, prefix := range []string{"dc", "opf"} {
					if strings.Count(output, "xmlns:"+prefix+"=") != 1 {
						t.Fatalf("inherited %s namespace was redeclared", prefix)
					}
				}
			}

			before, err := os.ReadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			changed, err = reader.WriteAuthorIDs(filename, ids)
			if err != nil || changed {
				t.Fatalf("repeated update: changed=%v, err=%v", changed, err)
			}
			after, err := os.ReadFile(filename)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("idempotent update rewrote the EPUB")
			}
			ids["douglas-adams"] = "Q99"
			if _, err := reader.WriteAuthorIDs(filename, ids); err != nil {
				t.Fatal(err)
			}
			info, err = epub.GetMetadataFromFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			assertAuthorID(t, info.Creator[0], "Q99", version)
			ids["douglas-adams"] = ""
			if _, err := reader.WriteAuthorIDs(filename, ids); err != nil {
				t.Fatal(err)
			}
			info, err = epub.GetMetadataFromFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			assertAuthorID(t, info.Creator[0], "", version)
			assertAuthorID(t, info.Creator[2], "Q1 Q2", version)
		})
	}
}

func testWriterPrefix(version string) string {
	if strings.HasPrefix(version, "3") {
		return ` prefix="other: https://example.org/other/"`
	}
	return ""
}

func assertAuthorID(t *testing.T, author epub.Author, want, version string) {
	t.Helper()
	count := 0
	for _, m := range author.Refinements {
		if m.Property == WikidataAuthorIDsProperty {
			if version != "3.0" {
				t.Fatal("EPUB 2 author ID must be a direct attribute, not a refinement")
			}
			count++
			if m.Value != want || m.Refines == "" || m.Refines == "#" {
				t.Fatalf("unexpected refinement for %s: %+v; want %q", author.FullName, m, want)
			}
		}
	}
	for _, a := range author.Attributes {
		if a.Name == (xml.Name{Local: WikidataAuthorIDAttribute}) {
			t.Fatal("obsolete unnamespaced wd_id attribute was retained")
		}
		if a.Name == (xml.Name{Space: CoreanderMetadataPrefix, Local: WikidataAuthorIDAttribute}) {
			if version != "2.0" {
				t.Fatal("EPUB 3 author ID must remain a refinement")
			}
			count++
			if a.Value != want {
				t.Fatalf("unexpected wd_id attribute for %s: %q; want %q", author.FullName, a.Value, want)
			}
		}
	}
	if (want == "" && count != 0) || (want != "" && count != 1) {
		t.Fatalf("got %d Wikidata IDs for %s; want %q", count, author.FullName, want)
	}
}

func TestWriteAuthorIDsRejectsConflictingPrefix(t *testing.T) {
	filename := writeTestEpub(t, map[string]string{
		"META-INF/container.xml": testContainerXML,
		"OEBPS/content.opf": `<package xmlns="http://www.idpf.org/2007/opf" version="3.0" prefix="cdr: https://example.org/conflict/">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:creator>Douglas Adams</dc:creator></metadata>
<manifest/><spine/></package>`,
	})
	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := NewEpubReader().WriteAuthorIDs(filename, map[string]string{"douglas-adams": "Q42"})
	if err == nil || changed {
		t.Fatalf("expected prefix conflict, got changed=%v, err=%v", changed, err)
	}
	after, err := os.ReadFile(filename)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("failed write modified the original file")
	}
}

func TestWriteAuthorIDsMigratesLegacyWrappers(t *testing.T) {
	for _, want := range []string{"Q42", "Q99", ""} {
		t.Run("ID="+want, func(t *testing.T) {
			filename := writeTestEpub(t, map[string]string{
				"META-INF/container.xml": testContainerXML,
				"OEBPS/content.opf": `<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:creator id="author" wd_id="Q42" custom="keep">Douglas Adams</dc:creator>
<dc:creator id="other">Other Author</dc:creator>
<meta name="epub:refinement" content='{"Property":"cdr:wd_id","Refines":"#author","Value":"Q42"}'/>
<meta name="epub:refinement" content='{"Property":"cdr:wd_author_ids","Refines":"#author","Value":"Q42"}'/>
<meta name="epub:refinement" content='{"Property":"cdr:wd_id","Refines":"#other","Value":"Q123"}'/>
<meta name="epub:refinement" content='{"Property":"custom:keep","Refines":"#author","Value":"keep"}'/>
<meta name="custom" content="keep"/>
</metadata><manifest/><spine/></package>`,
			})
			ids := map[string]string{"douglas-adams": want}
			changed, err := NewEpubReader().WriteAuthorIDs(filename, ids)
			if err != nil || !changed {
				t.Fatalf("migration: changed=%v, err=%v", changed, err)
			}
			info, err := epub.GetMetadataFromFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			assertAuthorID(t, info.Creator[0], want, "2.0")
			if len(info.Meta) != 3 {
				t.Fatalf("unrelated legacy metadata was removed: %+v", info.Meta)
			}
			for _, meta := range info.Meta {
				if strings.Contains(meta.Content, `"Property":"cdr:wd_id","Refines":"#author"`) {
					t.Fatal("obsolete author refinement wrapper was retained")
				}
			}
			changed, err = NewEpubReader().WriteAuthorIDs(filename, ids)
			if err != nil || changed {
				t.Fatalf("repeat migration: changed=%v, err=%v", changed, err)
			}
		})
	}
}

func TestWriteAuthorIDsMigratesPlainAttribute(t *testing.T) {
	for _, want := range []string{"Q42", "Q99", ""} {
		t.Run("ID="+want, func(t *testing.T) {
			filename := writeTestEpub(t, map[string]string{
				"META-INF/container.xml": testContainerXML,
				"OEBPS/content.opf": `<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:other="https://example.org/other/">
<dc:creator wd_id="Q42" other:wd_id="keep">Douglas Adams</dc:creator>
</metadata><manifest/><spine/></package>`,
			})
			ids := map[string]string{"douglas-adams": want}
			changed, err := NewEpubReader().WriteAuthorIDs(filename, ids)
			if err != nil || !changed {
				t.Fatalf("migration: changed=%v, err=%v", changed, err)
			}
			info, err := epub.GetMetadataFromFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			assertAuthorID(t, info.Creator[0], want, "2.0")
			preserved := false
			for _, attribute := range info.Creator[0].Attributes {
				if attribute.Name == (xml.Name{Space: "https://example.org/other/", Local: "wd_id"}) && attribute.Value == "keep" {
					preserved = true
				}
			}
			if !preserved {
				t.Fatal("unrelated namespace attribute was removed")
			}
			changed, err = NewEpubReader().WriteAuthorIDs(filename, ids)
			if err != nil || changed {
				t.Fatalf("repeat migration: changed=%v, err=%v", changed, err)
			}
		})
	}
}

func TestWriteAuthorIDsCleansCreatorNamespaces(t *testing.T) {
	filename := writeTestEpub(t, map[string]string{
		"META-INF/container.xml": testContainerXML,
		"OEBPS/content.opf": `<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
<dc:creator id="author" opf:role="aut" opf:file-as="Adams, Douglas"
xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf"
xmlns:vocab="https://github.com/svera/coreander/vocab/" vocab:wd_id="Q42">Douglas Adams</dc:creator>
</metadata><manifest/><spine/></package>`,
	})
	changed, err := NewEpubReader().WriteAuthorIDs(filename, map[string]string{"douglas-adams": "Q99"})
	if err != nil || !changed {
		t.Fatalf("update: changed=%v, err=%v", changed, err)
	}
	book, err := epub.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	opf, err := readZipFile(book.ReadCloser, "OEBPS/content.opf")
	if err != nil {
		t.Fatal(err)
	}
	output := string(opf)
	if !strings.Contains(output, `cdr:wd_id="Q99"`) || strings.Contains(output, "xmlns:vocab") {
		t.Fatal("old generated namespace prefix was not replaced")
	}
	for _, prefix := range []string{"dc", "opf", "cdr"} {
		if strings.Count(output, "xmlns:"+prefix+"=") != 1 {
			t.Fatalf("expected one %s namespace declaration: %s", prefix, output)
		}
	}
	creatorStart := strings.Index(output, "<dc:creator")
	if creatorStart < 0 {
		t.Fatal("creator element is missing")
	}
	creatorEnd := strings.Index(output[creatorStart:], ">") + creatorStart
	if strings.Contains(output[creatorStart:creatorEnd], "xmlns") {
		t.Fatal("namespace declarations must not appear on the creator")
	}
}

func TestBuildEpubMetadataOptionalText(t *testing.T) {
	info := &epub.Information{
		Title:       []epub.MetadataValue{{}},
		Description: []epub.MetadataValue{{}},
		Language:    []epub.MetadataValue{{}},
		Subject:     []epub.MetadataValue{{}, epub.NewMetadataValue(" Fiction;Science ")},
	}
	got, err := BuildEpubMetadataFields(info, "book.epub")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "book" || got.Description != "" || got.Language != "" ||
		!reflect.DeepEqual(got.Subjects, []string{"Fiction", "Science"}) {
		t.Fatalf("optional text was not handled safely: %+v", got)
	}
}

func TestBuildEpubMetadataOptionalSeries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		info   epub.Information
		series string
		index  float64
	}{
		{name: "absent values"},
		{
			name: "series and index",
			info: epub.Information{
				Series: epub.NewMetadataValue("Collection"), SeriesIndex: epub.NewMetadataValue("2.5"),
			},
			series: "Collection", index: 2.5,
		},
		{
			name:   "series without index",
			info:   epub.Information{Series: epub.NewMetadataValue("Collection")},
			series: "Collection",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildEpubMetadataFields(&tc.info, "book.epub")
			if err != nil {
				t.Fatal(err)
			}
			if got.Series != tc.series || got.SeriesIndex != tc.index {
				t.Fatalf("got series %q, index %v; want %q, %v", got.Series, got.SeriesIndex, tc.series, tc.index)
			}
		})
	}
}

func TestWriteAuthorIDsRejectsMalformedLegacyWrapper(t *testing.T) {
	filename := writeTestEpub(t, map[string]string{
		"META-INF/container.xml": testContainerXML,
		"OEBPS/content.opf": `<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:creator id="author">Douglas Adams</dc:creator>
<meta name="epub:refinement" content="invalid JSON"/>
</metadata><manifest/><spine/></package>`,
	})
	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := NewEpubReader().WriteAuthorIDs(filename, map[string]string{"douglas-adams": "Q42"})
	if err == nil || changed {
		t.Fatalf("expected legacy decoding error, got changed=%v, err=%v", changed, err)
	}
	after, err := os.ReadFile(filename)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("failed migration changed the original EPUB")
	}
}
