package webserver_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/PuerkitoBio/goquery"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	datasourcemodel "github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/datasource/wikidata"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/precisiondate"
	"github.com/svera/coreander/v5/internal/webserver"
	authorcontroller "github.com/svera/coreander/v5/internal/webserver/controller/author"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
)

type alternativeAuthorSource struct {
	authorcontroller.DataSource
	retrievedIDs []string
}

func (*alternativeAuthorSource) ProfileServiceName() string { return "Example catalog" }
func (*alternativeAuthorSource) ReferenceKey() string       { return "example" }
func (*alternativeAuthorSource) OwnsLink(key string) bool   { return key == "example" }
func (*alternativeAuthorSource) ProfileValue(author index.Author) string {
	return "https://example.org/authors/" + author.DataSourceReferences["example"]
}
func (*alternativeAuthorSource) AuthorLink(index.Author, string) datasourcemodel.DataSourceLink {
	return datasourcemodel.DataSourceLink{}
}

func (*alternativeAuthorSource) NormalizeAuthorID(value string) (string, error) {
	if value != "https://example.org/authors/alice" {
		return "", errors.New("invalid profile")
	}
	return "alice", nil
}

func (s *alternativeAuthorSource) RetrieveAuthor(ids []string, _ []string) (datasourcemodel.Author, error) {
	s.retrievedIDs = ids
	return alternativeAuthorMetadata{}, nil
}

type alternativeAuthorMetadata struct{}

func (alternativeAuthorMetadata) BirthName() string         { return "Alice Example" }
func (alternativeAuthorMetadata) Description(string) string { return "Updated author metadata" }
func (alternativeAuthorMetadata) InstanceOf() float64       { return 0 }
func (alternativeAuthorMetadata) Gender() float64           { return 0 }
func (alternativeAuthorMetadata) DateOfBirth() precisiondate.PrecisionDate {
	return precisiondate.PrecisionDate{}
}
func (alternativeAuthorMetadata) DateOfDeath() precisiondate.PrecisionDate {
	return precisiondate.PrecisionDate{}
}
func (alternativeAuthorMetadata) Image() string { return "" }
func (alternativeAuthorMetadata) Links() map[string]string {
	return map[string]string{"example": "alice"}
}
func (alternativeAuthorMetadata) OwnsLink(key string) bool { return key == "example" }
func (alternativeAuthorMetadata) RetrievedOn() time.Time   { return time.Unix(1, 0).UTC() }
func (alternativeAuthorMetadata) Pseudonyms() []string     { return nil }

func TestUpdateUsesDataSourceProfileParser(t *testing.T) {
	source := &alternativeAuthorSource{}
	appFS := loadDirInMemoryFs("testdata/library")
	cfg := defaultTestConfig()
	cfg.CacheDir = "/cache"
	app, idx := bootstrapAppWithAuthorSource(infrastructure.Connect(":memory:", 250), &infrastructure.NoEmail{}, appFS, cfg, source)
	admin, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	author, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	author.DataSourceReferences = map[string]string{"example": "old-profile", "independent": "https://other.example/author"}
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	if err := appFS.MkdirAll("/cache/authors", 0755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(appFS, "/cache/authors/john-doe.webp", []byte("cached image"), 0644); err != nil {
		t.Fatal(err)
	}
	before, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	response, err := putRequest(url.Values{"sourceID": {"invalid"}}, admin, app, "/authors/john-doe", t)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("expected bad request, got %d", response.StatusCode)
	}
	after, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || len(source.retrievedIDs) != 0 {
		t.Fatal("invalid input must not alter metadata or trigger a lookup")
	}
	if _, err := appFS.Stat("/cache/authors/john-doe.webp"); err != nil {
		t.Fatalf("invalid input must not remove the cached image: %v", err)
	}

	response, err = putRequest(url.Values{"sourceID": {"https://example.org/authors/alice"}}, &http.Cookie{Name: "anonymous"}, app, "/authors/john-doe", t)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode == http.StatusOK || len(source.retrievedIDs) != 0 {
		t.Fatal("anonymous visitors must not update author metadata")
	}

	for _, style := range []string{"", "?style=clear"} {
		t.Run("style="+style, func(t *testing.T) {
			source.retrievedIDs = nil
			response, err := putRequest(url.Values{"sourceID": {"https://example.org/authors/alice"}}, admin, app, "/authors/john-doe"+style, t)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			mustReturnStatus(response, http.StatusOK, t)
			page, err := goquery.NewDocumentFromReader(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if page.Find(`input[name="sourceID"]`).Length() != 1 {
				t.Fatal("updated author summary must include the correction form")
			}
			if value := page.Find(`input[name="sourceID"]`).AttrOr("value", ""); value != "https://example.org/authors/alice" {
				t.Fatalf("expected the provider profile URL in the form, got %q", value)
			}
			if description := strings.TrimSpace(page.Find("p.capitalize.lead").Text()); description != "Updated author metadata" {
				t.Fatalf("expected metadata rendered from the provider's reference, got %q", description)
			}
			if imageURL := page.Find(`img[src^="/authors/"]`).AttrOr("src", ""); imageURL != "/authors/john-doe_alice.webp" {
				t.Fatalf("expected image URL to use the provider's reference, got %q", imageURL)
			}
			stored, err := idx.Author("john-doe", "en")
			if err != nil {
				t.Fatal(err)
			}
			if stored.DataSourceReferences["example"] != "alice" || stored.BirthName != "Alice Example" ||
				stored.Description["en"] != "Updated author metadata" ||
				stored.DataSourceReferences["independent"] != before.DataSourceReferences["independent"] ||
				!reflect.DeepEqual(source.retrievedIDs, []string{"alice"}) {
				t.Fatalf("expected normalized ID retrieval and persisted metadata while preserving independent links: %+v", stored)
			}
		})
	}
}

func TestClearAuthorRemovesImage(t *testing.T) {
	mockDataSourceServer := wikidata.NewMockServer(t, "fixtures/datasource/wikidata")
	gowikidata.WikidataDomain = mockDataSourceServer.URL
	defer mockDataSourceServer.Close()

	defaultImage, err := os.ReadFile("embedded/images/male.webp")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		style  string
		cached bool
		input  string
	}{
		{"summary with cached image", "", true, ""},
		{"document detail with cached image", "?style=clear", true, "  "},
		{"summary without cached image", "", false, ""},
		{"document detail without cached image", "?style=clear", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := infrastructure.Connect(":memory:", 250)
			appFS := loadDirInMemoryFs("testdata/library")
			cfg := defaultTestConfig()
			cfg.CacheDir = "/cache"
			app, idx := bootstrapAppWithIndexer(db, &infrastructure.NoEmail{}, appFS, cfg)

			cookie, err := login(app, "admin@example.com", "admin", t)
			if err != nil {
				t.Fatal(err)
			}
			response, err := getRequest(cookie, app, "/authors/john-doe/summary", t)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			mustReturnStatus(response, http.StatusOK, t)

			author, err := idx.Author("john-doe", "en")
			if err != nil {
				t.Fatal(err)
			}
			author.DataSourceReferences = map[string]string{
				"website": "https://old.example", "wikipedia:en": "https://en.wikipedia.org/wiki/Old",
				"wikipedia:es": "https://es.wikipedia.org/wiki/Old",
				"goodreads":    "1.Writer", "catalog": "https://catalog.example/writer",
			}
			if err := idx.IndexAuthor(author); err != nil {
				t.Fatal(err)
			}
			documentCount := author.DocumentCount

			imagePath := cfg.CacheDir + "/authors/john-doe.webp"
			if tc.cached {
				if err := appFS.MkdirAll(cfg.CacheDir+"/authors", 0755); err != nil {
					t.Fatal(err)
				}
				if err := afero.WriteFile(appFS, imagePath, []byte("old author image"), 0644); err != nil {
					t.Fatal(err)
				}
			}

			response, err = putRequest(url.Values{"sourceID": {tc.input}}, cookie, app, "/authors/john-doe"+tc.style, t)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			mustReturnStatus(response, http.StatusOK, t)
			if _, err := appFS.Stat(imagePath); !os.IsNotExist(err) {
				t.Fatalf("expected cached image to be absent, got %v", err)
			}

			doc, err := goquery.NewDocumentFromReader(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			imageURL, _ := doc.Find(`img[src^="/authors/"]`).Attr("src")
			if imageURL != "/authors/john-doe_.webp" {
				t.Fatalf("expected image URL with cleared ID and no cached version, got %q", imageURL)
			}
			if doc.Find("dt:contains('Date of birth')").Length() != 0 {
				t.Fatal("expected author information to be cleared")
			}

			response, err = getRequest(cookie, app, imageURL, t)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			mustReturnStatus(response, http.StatusOK, t)
			imageData, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(imageData, defaultImage) {
				t.Fatal("expected default author image after clearing Wikidata ID")
			}

			response, err = getRequest(cookie, app, "/authors/john-doe/summary"+tc.style, t)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			mustReturnStatus(response, http.StatusOK, t)
			doc, err = goquery.NewDocumentFromReader(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if imageURL, _ := doc.Find(`img[src^="/authors/"]`).Attr("src"); imageURL != "/authors/john-doe_.webp" {
				t.Fatalf("expected cleared image URL to persist, got %q", imageURL)
			}
			stored, err := idx.Author("john-doe", "en")
			if err != nil {
				t.Fatal(err)
			}
			if stored.DataSourceReferences["wikidata"] != "" || stored.DataSourceImage != "" ||
				stored.DataSourceReferences["website"] != "" || stored.DataSourceReferences["wikipedia:en"] != "" ||
				stored.DataSourceReferences["wikipedia:es"] != "" {
				t.Fatalf("metadata reset restored stale metadata: %+v", stored)
			}
			if stored.DataSourceReferences["goodreads"] != "1.Writer" ||
				stored.DataSourceReferences["catalog"] != "https://catalog.example/writer" ||
				stored.DocumentCount != documentCount {
				t.Fatalf("metadata reset lost independent author data: %+v", stored)
			}
		})
	}
}

type summaryDataSourceLinks struct{}

func (summaryDataSourceLinks) AuthorLink(author index.Author, _ string) datasourcemodel.DataSourceLink {
	if author.DataSourceReferences["wikidata"] == "" {
		return datasourcemodel.DataSourceLink{}
	}
	return datasourcemodel.DataSourceLink{
		Source: "Example catalog", URL: "https://example.org/authors/" + author.Slug,
		IconClass: "bi-bookmark",
	}
}

func TestAuthorSummaryDataSourceLinks(t *testing.T) {
	app, idx := newDataSourceProvidersTestApp(t, nil, nil, summaryDataSourceLinks{})
	const (
		authorURL    = "https://example.org/authors/"
		wikipediaURL = "https://en.wikipedia.org/wiki/Alice_Borchardt"
	)
	locales := []string{"en", "es", "de", "fr", "ru"}
	for _, tc := range []struct {
		name          string
		dataSourceID  string
		hasDataSource bool
		wikipedia     bool
	}{
		{name: "with-wikipedia", dataSourceID: "Q1", hasDataSource: true, wikipedia: true},
		{name: "without-wikipedia", dataSourceID: "Q1", hasDataSource: true},
		{name: "without-data-source-id"},
	} {
		author := index.Author{
			Slug: tc.name, Name: "Alice Borchardt",
			RetrievedOn:          time.Now().UTC(),
			DataSourceReferences: map[string]string{"wikidata": tc.dataSourceID, "website": "https://author.example/" + tc.name},
		}
		if tc.wikipedia {
			for _, lang := range locales {
				author.DataSourceReferences["wikipedia:"+lang] = strings.Replace(wikipediaURL, "en.wikipedia.org", lang+".wikipedia.org", 1)
			}
		}
		if err := idx.IndexAuthor(author); err != nil {
			t.Fatal(err)
		}
		for _, style := range []struct {
			name  string
			query string
		}{
			{"summary", ""},
			{"document-detail", "?style=clear"},
		} {
			for _, lang := range locales {
				t.Run(tc.name+"/"+style.name+"/"+lang, func(t *testing.T) {
					req := mustGetRequest(t, "/authors/"+tc.name+"/summary"+style.query)
					req.Header.Set("Accept-Language", lang)
					response, err := app.Test(req)
					if err != nil {
						t.Fatal(err)
					}
					defer response.Body.Close()
					mustReturnStatus(response, http.StatusOK, t)
					page, err := goquery.NewDocumentFromReader(response.Body)
					if err != nil {
						t.Fatal(err)
					}
					wikipediaLink := page.Find(`a[href="https://` + lang + `.wikipedia.org/wiki/Alice_Borchardt"]`)
					websiteLink := page.Find(`a[href="https://author.example/` + tc.name + `"]`)
					if websiteLink.Length() != 1 || websiteLink.PrevFiltered("i.bi-globe-europe-africa").Length() != 1 {
						t.Fatal("expected the stored website link and globe icon")
					}
					if tc.wikipedia && (wikipediaLink.Length() != 1 || strings.TrimSpace(wikipediaLink.Text()) != "Wikipedia") {
						t.Fatalf("expected Wikipedia data-source link, got %d links with text %q", wikipediaLink.Length(), wikipediaLink.Text())
					}
					if tc.wikipedia && wikipediaLink.PrevFiltered("i.bi-wikipedia").Length() != 1 {
						t.Fatal("expected the Wikipedia provider's Bootstrap icon")
					}
					if !tc.wikipedia && page.Find(`a[href$="wikipedia.org/wiki/Alice_Borchardt"]`).Length() != 0 {
						t.Fatal("expected no Wikipedia link when there is no link for the current language")
					}
					link := page.Find(`a[href="` + authorURL + tc.name + `"]`)
					if !tc.hasDataSource {
						if link.Length() != 0 {
							t.Fatal("expected no data-source link without a provider URL")
						}
						return
					}
					if link.Length() != 1 || strings.TrimSpace(link.Text()) != "Example catalog" {
						t.Fatalf("expected one Example catalog data-source link, got %d links with text %q", link.Length(), link.Text())
					}
					if link.PrevFiltered("i.bi-bookmark").Length() != 1 {
						t.Error("expected the provider's Bootstrap icon before the data-source link")
					}
					if tc.wikipedia {
						wantWikipediaURL := strings.Replace(wikipediaURL, "en.wikipedia.org", lang+".wikipedia.org", 1)
						if href, _ := link.Parent().Prev().Find("a").Attr("href"); href != wantWikipediaURL {
							t.Fatalf("expected data-source link immediately below Wikipedia, previous link is %q", href)
						}
					}
				})
			}
		}
	}
}

type alternativeDataSourceLinks struct{}

func (alternativeDataSourceLinks) AuthorLink(author index.Author, _ string) datasourcemodel.DataSourceLink {
	return datasourcemodel.DataSourceLink{Source: "Another source", URL: "https://example.org/authors/" + author.Slug}
}

type editableAlternativeLinks struct{}

func (editableAlternativeLinks) AuthorLink(author index.Author, _ string) datasourcemodel.DataSourceLink {
	return datasourcemodel.DataSourceLink{
		Source: "Example catalog", URL: "https://example.org/authors/" + author.Slug,
	}
}

func (editableAlternativeLinks) ProfileServiceName() string       { return "Example catalog" }
func (editableAlternativeLinks) ReferenceKey() string             { return "example" }
func (editableAlternativeLinks) ProfileValue(index.Author) string { return "catalog-profile" }

func TestDocumentAuthorProfileUsesMetadataSource(t *testing.T) {
	app, idx := newDataSourceProvidersTestApp(t, nil, nil, editableAlternativeLinks{})
	if err := idx.IndexAuthor(index.Author{
		Slug: "jane-doe", Name: "Jane Doe", DataSourceReferences: map[string]string{"wikidata": "Q1234"}, RetrievedOn: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	admin, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/authors/jane-doe/summary",
		"/authors/jane-doe/summary?style=clear",
		"/documents/unlinked",
	} {
		response, err := getRequest(admin, app, path, t)
		if err != nil {
			t.Fatal(err)
		}
		page, err := goquery.NewDocumentFromReader(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		link := page.Find(`a[href="https://example.org/authors/jane-doe"]`)
		if link.Length() != 1 || link.Text() != "Example catalog" {
			t.Fatalf("%s: expected provider label Example catalog, got %q", path, link.Text())
		}
		if path == "/documents/unlinked" {
			form := page.Find(`#update-author-form-jane-doe`)
			input := form.Find(`form[hx-put="/authors/jane-doe?style=clear"] input[name="sourceID"]`)
			if input.Length() != 1 || form.Find(`input[name="sourceID"]`).Length() != 1 ||
				input.AttrOr("value", "") != "https://www.wikidata.org/wiki/Q1234" ||
				input.AttrOr("aria-label", "") != "Wikidata profile" {
				t.Fatal("document detail must edit only the configured metadata source, not a link provider")
			}
		}
	}
}

func TestAlternativeDataSourceLinks(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{
		Rating: "4", ResourceID: "work", LegacyBookID: 1,
	}}
	app, idx := newDataSourceProvidersTestApp(t, source, nil, alternativeDataSourceLinks{})
	for _, slug := range []string{"jane-doe", "john-doe"} {
		if err := idx.IndexAuthor(index.Author{
			Slug: slug, Name: slug, RetrievedOn: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		"/authors/jane-doe",
		"/authors/jane-doe/summary",
		"/authors/jane-doe/summary?style=clear",
		"/documents/unlinked",
	} {
		page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, path)))
		if err != nil {
			t.Fatal(err)
		}
		link := page.Find(`a[href="https://example.org/authors/jane-doe"]`)
		if link.Length() != 1 || link.Text() != "Another source" {
			t.Fatalf("%s: expected injected author data-source link Another source, got %q", path, link.Text())
		}
		if page.Find(`img[src^="/images/goodreads.svg"]`).Length() != 0 {
			t.Fatalf("%s: unexpected Goodreads branding", path)
		}
	}
	for path, wantLinks := range map[string]int{"/documents/linked/score": 2, "/documents/linked/reviews": 1} {
		page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, path)))
		if err != nil {
			t.Fatal(err)
		}
		if page.Find(`a[href="https://www.goodreads.com/book/show/12207444-la-reina-dragon"]`).Length() != wantLinks {
			t.Fatalf("%s: BiblioReads must retain its Goodreads document URL regardless of the author link provider", path)
		}
	}
}

func TestDataSourceLinksWithoutReviewProvider(t *testing.T) {
	app, idx := newDataSourceProvidersTestApp(t, nil, nil, alternativeDataSourceLinks{})
	if err := idx.IndexAuthor(index.Author{Slug: "jane-doe", Name: "Jane Doe", RetrievedOn: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	body := getBody(t, app, "/authors/jane-doe/summary")
	if !strings.Contains(body, "https://example.org/authors/jane-doe") {
		t.Fatal("links must not require a review provider")
	}
}

func TestAuthorSummary(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	appFS := loadDirInMemoryFs("testdata/library")
	mockDataSourceServer := wikidata.NewMockServer(t, "fixtures/datasource/wikidata")

	gowikidata.WikidataDomain = mockDataSourceServer.URL

	defer mockDataSourceServer.Close()

	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	var cases = []struct {
		name                string
		url                 string
		expectedStatus      int
		expectedDateOfBirth int
	}{
		{"Search for authors", "/authors/john-doe/summary", http.StatusOK, 1},
		{"Search for authors not indexed", "/authors/not-found/summary", http.StatusNotFound, 0},
		{"Search for authors indexed but not in Wikidata", "/authors/miguel-de-cervantes-y-saavedra/summary", http.StatusOK, 0},
		{"Document summary for authors not in Wikidata", "/authors/miguel-de-cervantes-y-saavedra/summary?style=clear", http.StatusOK, 0},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tcase.url, nil)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
			req.Header.Set("Accept-Language", "en")
			response, err := app.Test(req)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
			if expectedStatus := tcase.expectedStatus; response.StatusCode != expectedStatus {
				t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
			}

			doc, err := goquery.NewDocumentFromReader(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if actualResults := doc.Find("dt:contains('Date of birth')").Length(); actualResults != tcase.expectedDateOfBirth {
				t.Errorf("Expected %d results, got %d", tcase.expectedDateOfBirth, actualResults)
			}
			if strings.Contains(tcase.url, "miguel-de-cervantes") {
				if doc.Find("#author-miguel-de-cervantes-y-saavedra").Length() != 1 ||
					!strings.Contains(doc.Text(), "Sorry, we don't have information about this author at the moment") {
					t.Error("expected an author summary explaining that information is unavailable")
				}
			}
		})
	}
}
