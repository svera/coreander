package webserver_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	datasourcemodel "github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/datasource/wikidata"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/webserver"
	authorcontroller "github.com/svera/coreander/v5/internal/webserver/controller/author"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
)

type alternativeAuthorSource struct {
	authorcontroller.DataSource
	retrievedIDs []string
}

func (*alternativeAuthorSource) NormalizeAuthorID(value string) (string, error) {
	if value != "https://example.org/authors/alice" {
		return "", errors.New("invalid profile")
	}
	return "alice", nil
}

func (s *alternativeAuthorSource) RetrieveAuthor(ids []string, _ []string) (datasourcemodel.Author, error) {
	s.retrievedIDs = ids
	return nil, nil
}

type existingAuthorIndex struct{ authorcontroller.IdxReader }

func (existingAuthorIndex) Author(slug, _ string) (index.Author, error) {
	return index.Author{Slug: slug}, nil
}

func TestUpdateUsesDataSourceProfileParser(t *testing.T) {
	source := &alternativeAuthorSource{}
	controller := authorcontroller.NewController(nil, nil, nil, existingAuthorIndex{}, authorcontroller.Config{}, source, nil, nil)
	app := fiber.New()
	app.Put("/authors/:slug", func(c fiber.Ctx) error {
		c.Locals("Lang", "en")
		c.Locals("SupportedLanguages", []string{"en"})
		return controller.Update(c)
	})

	for _, tc := range []struct {
		input      string
		wantStatus int
	}{
		{"https://example.org/authors/alice", http.StatusNotFound},
		{"invalid", http.StatusBadRequest},
	} {
		source.retrievedIDs = nil
		req := httptest.NewRequest(http.MethodPut, "/authors/test", strings.NewReader(url.Values{"sourceID": {tc.input}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.wantStatus {
			t.Fatalf("%q: status = %d, want %d", tc.input, resp.StatusCode, tc.wantStatus)
		}
		if tc.wantStatus == http.StatusNotFound && (len(source.retrievedIDs) != 1 || source.retrievedIDs[0] != "alice") {
			t.Fatalf("normalized source ID was not used for retrieval: %v", source.retrievedIDs)
		}
		if tc.wantStatus == http.StatusBadRequest && len(source.retrievedIDs) != 0 {
			t.Fatal("invalid input must not trigger retrieval")
		}
	}
}

func TestUpdateAuthorWikidataInput(t *testing.T) {
	mock := wikidata.NewMockServer(t, "fixtures/datasource/wikidata")
	defer mock.Close()
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		mock.Config.Handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	previousDomain := gowikidata.WikidataDomain
	gowikidata.WikidataDomain = server.URL
	defer func() { gowikidata.WikidataDomain = previousDomain }()

	appFS := loadDirInMemoryFs("testdata/library")
	cfg := defaultTestConfig()
	cfg.CacheDir = "/cache"
	app, idx := bootstrapAppWithIndexer(infrastructure.Connect(":memory:", 250), &infrastructure.NoEmail{}, appFS, cfg)
	admin, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	author, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	author.DataSourceID = "Q999"
	author.DataSourceLinks = map[string]string{"goodreads": "1.Writer"}
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
	initialRequests := requests.Load()
	for _, input := range []string{
		"https://evil.example/wiki/Q1234", "https://www.wikidata.org/wiki/Q1234?x=1",
		"https://www.wikidata.org/wiki/Q1234/extra", "P1234",
	} {
		response, err := putRequest(url.Values{"sourceID": {input}}, admin, app, "/authors/john-doe", t)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("%q: expected bad request, got %d", input, response.StatusCode)
		}
	}
	after, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || requests.Load() != initialRequests {
		t.Fatal("invalid input must not alter metadata or trigger a lookup")
	}
	if _, err := appFS.Stat("/cache/authors/john-doe.webp"); err != nil {
		t.Fatalf("invalid input must not remove the cached image: %v", err)
	}

	response, err := putRequest(url.Values{"sourceID": {"Q1234"}}, &http.Cookie{Name: "anonymous"}, app, "/authors/john-doe", t)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode == http.StatusOK || requests.Load() != initialRequests {
		t.Fatal("anonymous visitors must not update author metadata")
	}

	for _, tc := range []struct{ input, style string }{
		{"Q1234", ""},
		{"q1234", "?style=clear"},
		{"https://www.wikidata.org/wiki/Q1234", ""},
		{"http://wikidata.org/entity/q1234/", "?style=clear"},
	} {
		t.Run(tc.input+tc.style, func(t *testing.T) {
			beforeRequests := requests.Load()
			response, err := putRequest(url.Values{"sourceID": {tc.input}}, admin, app, "/authors/john-doe"+tc.style, t)
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
			if value := page.Find(`input[name="sourceID"]`).AttrOr("value", ""); value != "https://www.wikidata.org/wiki/Q1234" {
				t.Fatalf("expected the Wikidata profile URL in the form, got %q", value)
			}
			stored, err := idx.Author("john-doe", "en")
			if err != nil {
				t.Fatal(err)
			}
			if stored.DataSourceID != "Q1234" || stored.DataSourceLinks["goodreads"] != "1.Writer" || requests.Load() <= beforeRequests {
				t.Fatalf("expected a lookup and normalized Wikidata ID while preserving Goodreads: %+v", stored)
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
			author.DataSourceLinks = map[string]string{
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
			if stored.DataSourceID != "" || stored.DataSourceImage != "" ||
				stored.DataSourceLinks["website"] != "" || stored.DataSourceLinks["wikipedia:en"] != "" ||
				stored.DataSourceLinks["wikipedia:es"] != "" {
				t.Fatalf("metadata reset restored stale metadata: %+v", stored)
			}
			if stored.DataSourceLinks["goodreads"] != "1.Writer" ||
				stored.DataSourceLinks["catalog"] != "https://catalog.example/writer" ||
				stored.DocumentCount != documentCount {
				t.Fatalf("metadata reset lost independent author data: %+v", stored)
			}
		})
	}
}

type translatedDataSourceLinks struct{}

func (translatedDataSourceLinks) AuthorLink(author index.Author, _ string) datasourcemodel.DataSourceLink {
	if author.DataSourceID == "" {
		return datasourcemodel.DataSourceLink{}
	}
	return datasourcemodel.DataSourceLink{
		Source: "Example catalog", URL: "https://example.org/authors/" + author.Slug,
		IconClass: "bi-bookmark",
	}
}

func TestAuthorSummaryDataSourceLinkTranslations(t *testing.T) {
	app, idx := newDataSourceProvidersTestApp(t, nil, nil, translatedDataSourceLinks{})
	const (
		authorURL    = "https://example.org/authors/"
		wikipediaURL = "https://en.wikipedia.org/wiki/Alice_Borchardt"
	)
	locales := []struct {
		lang           string
		label          string
		wikipediaLabel string
	}{
		{"en", "Go to Example catalog author page", "Go to Wikipedia entry"},
		{"es", "Ir a la página del autor en Example catalog", "Ir al artículo en Wikipedia"},
		{"de", "Zur Autorenseite auf Example catalog", "Zur Wikipedia-Seite"},
		{"fr", "Voir la page de l'auteur sur Example catalog", "Aller à l'entrée Wikipedia"},
		{"ru", "Перейти на страницу автора в Example catalog", "Перейти к статье в Википедии"},
	}
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
			DataSourceID: tc.dataSourceID, RetrievedOn: time.Now().UTC(),
			DataSourceLinks: map[string]string{"website": "https://author.example/" + tc.name},
		}
		if tc.wikipedia {
			for _, locale := range locales {
				author.DataSourceLinks["wikipedia:"+locale.lang] = strings.Replace(wikipediaURL, "en.wikipedia.org", locale.lang+".wikipedia.org", 1)
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
			for _, locale := range locales {
				t.Run(tc.name+"/"+style.name+"/"+locale.lang, func(t *testing.T) {
					req := mustGetRequest(t, "/authors/"+tc.name+"/summary"+style.query)
					req.Header.Set("Accept-Language", locale.lang)
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
					wikipediaLink := page.Find(`a[href="https://` + locale.lang + `.wikipedia.org/wiki/Alice_Borchardt"]`)
					websiteLink := page.Find(`a[href="https://author.example/` + tc.name + `"]`)
					if websiteLink.Length() != 1 || websiteLink.PrevFiltered("i.bi-globe-europe-africa").Length() != 1 {
						t.Fatal("expected the stored website link and globe icon")
					}
					wikipediaLabel := locale.wikipediaLabel
					if style.name == "summary" {
						wikipediaLabel = "Wikipedia"
					}
					if tc.wikipedia && (wikipediaLink.Length() != 1 || strings.TrimSpace(wikipediaLink.Text()) != wikipediaLabel) {
						t.Fatalf("expected Wikipedia data-source link %q, got %d links with text %q", wikipediaLabel, wikipediaLink.Length(), wikipediaLink.Text())
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
					if link.Length() != 1 || strings.TrimSpace(link.Text()) != locale.label {
						t.Fatalf("expected one localized data-source link %q, got %d links with text %q", locale.label, link.Length(), link.Text())
					}
					if link.PrevFiltered("i.bi-bookmark").Length() != 1 {
						t.Error("expected the provider's Bootstrap icon before the data-source link")
					}
					if tc.wikipedia {
						wantWikipediaURL := strings.Replace(wikipediaURL, "en.wikipedia.org", locale.lang+".wikipedia.org", 1)
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
		ShortLabel: "Catalog",
	}
}

func (editableAlternativeLinks) ProfileServiceName() string       { return "Example catalog" }
func (editableAlternativeLinks) ProfileValue(index.Author) string { return "catalog-profile" }

func TestDocumentAuthorProfileUsesMetadataSource(t *testing.T) {
	app, idx := newDataSourceProvidersTestApp(t, nil, nil, editableAlternativeLinks{})
	if err := idx.IndexAuthor(index.Author{
		Slug: "jane-doe", Name: "Jane Doe", DataSourceID: "Q1234", RetrievedOn: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	admin, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, label string
	}{
		{"/authors/jane-doe/summary", "Catalog"},
		{"/authors/jane-doe/summary?style=clear", "Go to Example catalog author page"},
		{"/documents/unlinked", "Go to Example catalog author page"},
	} {
		response, err := getRequest(admin, app, tc.path, t)
		if err != nil {
			t.Fatal(err)
		}
		page, err := goquery.NewDocumentFromReader(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		link := page.Find(`a[href="https://example.org/authors/jane-doe"]`)
		if link.Length() != 1 || link.Text() != tc.label {
			t.Fatalf("%s: expected provider label %q, got %q", tc.path, tc.label, link.Text())
		}
		if tc.path == "/documents/unlinked" {
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
		"/authors/jane-doe", "/authors/jane-doe/summary",
		"/authors/jane-doe/summary?style=clear", "/documents/unlinked",
	} {
		page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, path)))
		if err != nil {
			t.Fatal(err)
		}
		link := page.Find(`a[href="https://example.org/authors/jane-doe"]`)
		if link.Length() != 1 || link.Text() != "Go to Another source author page" {
			t.Fatalf("%s: expected injected author data-source link, got %q", path, link.Text())
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
