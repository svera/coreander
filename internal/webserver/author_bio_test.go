package webserver_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/biography"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	datasourcemodel "github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/precisiondate"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
	gt "gopkg.gilang.dev/translator/v2"
)

type bioAuthorClient struct {
	calls int
	desc  string
}

func (c *bioAuthorClient) Author(context.Context, string) (biblioreads.AuthorResponse, error) {
	c.calls++
	if c.desc != "" {
		return biblioreads.AuthorResponse{Desc: c.desc}, nil
	}
	return biblioreads.AuthorResponse{Desc: "John Doe is an American novelist who has written many books about the sea.<br><br>He lives in Maine with his family and his three dogs."}, nil
}

func (c *bioAuthorClient) RefreshAuthor(ctx context.Context, id string) (biblioreads.AuthorResponse, error) {
	return c.Author(ctx, id)
}

type bioTranslator struct{ calls int }

func (t *bioTranslator) Translate(ctx context.Context, text, from, to string) (*gt.Translated, error) {
	return t.TranslateWithGender(ctx, text, from, to, gt.GenderUnknown)
}

func (t *bioTranslator) TranslateWithGender(_ context.Context, text, from, to string, _ gt.Gender) (*gt.Translated, error) {
	t.calls++
	return &gt.Translated{
		Text: strings.ReplaceAll(text, "John Doe", "John Doe ("+to+")"),
		From: gt.TranslateFrom{Language: gt.TranslateFromLanguage{Iso: from}},
	}, nil
}

// lazyBioIndex lets the provider be configured before the index is created.
type lazyBioIndex struct{ idx *index.BleveIndexer }

func (l *lazyBioIndex) StoreAuthorBioForLink(slug, linkKey, expectedLink string, bio index.AuthorBio, merge bool) error {
	return l.idx.StoreAuthorBioForLink(slug, linkKey, expectedLink, bio, merge)
}

func (l *lazyBioIndex) StoreCanonicalAuthorBio(slug, linkKey, previousLink string, bio index.AuthorBio) error {
	return l.idx.StoreCanonicalAuthorBio(slug, linkKey, previousLink, bio)
}

func newAuthorBioTestApp(t *testing.T, client *bioAuthorClient, translator gt.Translator) (*fiber.App, *index.BleveIndexer) {
	t.Helper()
	return newAuthorBioTestAppWithSource(t, biblioreads.NewBioSource(client), translator, nil)
}

func newAuthorBioTestAppWithSource(t *testing.T, source biography.Source, translator gt.Translator, metadataSource *bioMetadataSource) (*fiber.App, *index.BleveIndexer) {
	t.Helper()
	bioIndex := &lazyBioIndex{}
	cfg := defaultTestConfig()
	if source != nil {
		cfg.AuthorBioProvider = biography.NewService(source, translator, bioIndex)
	}
	db := infrastructure.Connect(":memory:", 250)
	sender := &infrastructure.NoEmail{}
	appFS := loadDirInMemoryFs("testdata/library")
	var app *fiber.App
	var idx *index.BleveIndexer
	if metadataSource == nil {
		app, idx = bootstrapAppWithIndexer(db, sender, appFS, cfg)
	} else {
		app, idx = bootstrapAppWithAuthorSource(db, sender, appFS, cfg, metadataSource)
	}
	bioIndex.idx = idx
	return app, idx
}

func seedBioTestAuthor(t *testing.T, idx *index.BleveIndexer, links map[string]string) index.Author {
	t.Helper()
	author, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	author.DataSourceReferences = links
	author.RetrievedOn = time.Now()
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	return author
}

func getPageWithCookie(t *testing.T, app *fiber.App, cookie *http.Cookie, path string) *goquery.Document {
	t.Helper()
	response, err := getRequest(cookie, app, path, t)
	if err != nil {
		t.Fatal(err)
	}
	mustReturnStatus(response, http.StatusOK, t)
	return parseHTMLResponse(t, response)
}

func parseHTMLResponse(t *testing.T, response *http.Response) *goquery.Document {
	t.Helper()
	defer response.Body.Close()
	page, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

type alternateProfileInput struct{ biography.Source }

func (alternateProfileInput) ProfileInputName() string   { return "otherProfileID" }
func (alternateProfileInput) ProfileServiceName() string { return "Other service" }

func testAuthorBioAddProfile(t *testing.T) {
	client := &bioAuthorClient{}
	source := alternateProfileInput{biblioreads.NewBioSource(client)}
	app, idx := newAuthorBioTestAppWithSource(t, source, nil, nil)
	author := seedBioTestAuthor(t, idx, nil)
	admin, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	page := getPageWithCookie(t, app, admin, "/authors/john-doe/summary")
	if page.Find(`input[name="otherProfileID"][aria-label="Other service profile"]`).Length() != 1 ||
		client.calls != 0 {
		t.Fatal("profile input must use the biography provider's name and label")
	}
	for _, tc := range []struct {
		input      string
		wantStatus int
	}{
		{"invalid", http.StatusBadRequest},
		{"879", http.StatusOK},
	} {
		response, err := postRequest(url.Values{"otherProfileID": {tc.input}}, admin, app, "/authors/john-doe/bio", t)
		if err != nil {
			t.Fatal(err)
		}
		page = parseHTMLResponse(t, response)
		if response.StatusCode != tc.wantStatus {
			t.Fatalf("%q: status = %d, want %d", tc.input, response.StatusCode, tc.wantStatus)
		}
		if tc.wantStatus == http.StatusBadRequest &&
			page.Find(`input[name="otherProfileID"][aria-invalid="true"]`).AttrOr("value", "") != tc.input {
			t.Fatal("invalid provider-specific input must be preserved")
		}
	}
	author, err = idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	if source.ProfileValue(author) == "" || author.Bio.Source != "879" ||
		author.Bio.Text["en"] == "" || client.calls != 1 {
		t.Fatal("adding a profile must store its link and biography")
	}
}

func TestAuthorDetailCardLayoutAndSanitization(t *testing.T) {
	app, idx := newAuthorBioTestApp(t, &bioAuthorClient{}, nil)
	author, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	author.Description = map[string]string{"en": "American novelist"}
	author.RetrievedOn = time.Now()
	author.DateOfBirth = precisiondate.NewPrecisionDate("+1920-01-02T00:00:00Z", precisiondate.PrecisionDay)
	author.DateOfDeath = precisiondate.NewPrecisionDate("+1992-04-06T00:00:00Z", precisiondate.PrecisionDay)
	author.DataSourceReferences = map[string]string{
		"wikidata":  "Q1234",
		"goodreads": "1.John_Doe",
		"website":   "https://example.com",
	}
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	if err := idx.StoreAuthorBio(author.Slug, index.AuthorBio{
		Source: author.DataSourceReferences["goodreads"], Language: "en",
		Text: map[string]string{"en": `<b onclick="bad()">John Doe</b> wrote many <i>books</i>.<script>alert(1)</script>`},
	}); err != nil {
		t.Fatal(err)
	}
	admin, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/authors/john-doe?l=en", "/authors/john-doe/summary?l=en"} {
		t.Run(path, func(t *testing.T) {
			page := getPageWithCookie(t, app, admin, path)
			card := page.Find("#author-john-doe")
			main := card.Find(".author-details-main")
			sidebar := card.Find(".author-details-sidebar")
			if !main.Find(".lead").Next().Is("dl") ||
				main.Find("dl").NextAllFiltered("#author-bio").Length() != 1 {
				t.Fatal("birth and death dates must appear directly below the short description and before the biography")
			}
			if strings.TrimSpace(main.Find("#author-bio-text").Text()) != "John Doe wrote many books." ||
				main.Find("img, ul").Length() != 0 ||
				main.Find(`time[datetime="+001920-01-02"]`).Length() != 1 ||
				main.Find(`time[datetime="+001992-04-06"]`).Length() != 1 {
				t.Fatal("the left column must contain the biography and dates, not the photo or links")
			}
			if main.Find("#author-bio-text b").Text() != "John Doe" ||
				main.Find("#author-bio-text i").Text() != "books" ||
				main.Find("#author-bio-text script, #author-bio-text [onclick]").Length() != 0 {
				t.Fatal("biographies must render safe bold and italic markup only")
			}
			if !main.Next().Is(".author-details-sidebar") ||
				!main.HasClass("col-12") || !main.HasClass("col-md-8") ||
				!sidebar.HasClass("col-12") || !sidebar.HasClass("col-md-4") {
				t.Fatal("columns must be side by side from medium screens and stack on small screens")
			}
			if sidebar.Find(`img.author-image-upload[data-author-slug="john-doe"]`).Length() != 1 ||
				sidebar.Find("dl, time").Length() != 0 ||
				sidebar.Find(`a[href="https://example.com"]`).Length() != 1 ||
				sidebar.Find(`a[href="https://www.goodreads.com/author/show/1.John_Doe"]`).Length() != 1 {
				html, err := sidebar.Html()
				t.Fatalf("the right column must contain only the photo and external links: %s (%v)", html, err)
			}
			if sidebar.Find(`a[href="https://www.goodreads.com/author/show/1.John_Doe"]`).Text() != "Goodreads" {
				t.Fatal("the Goodreads link must use only the site name")
			}
		})
	}
}

func TestAuthorBio(t *testing.T) {
	client, translator := &bioAuthorClient{}, &bioTranslator{}
	app, idx := newAuthorBioTestApp(t, client, translator)

	page := getPage(t, app, "/authors/john-doe")
	if page.Find(`[hx-get="/authors/john-doe/bio"]`).Length() != 0 {
		t.Fatal("authors without Goodreads link must not request a bio")
	}

	seedBioTestAuthor(t, idx, map[string]string{"goodreads": "1.John_Doe"})
	summary := getPage(t, app, "/authors/john-doe/summary?l=es")
	if summary.Find(`#author-john-doe [hx-get="/authors/john-doe/bio"]`).Length() != 1 {
		t.Fatal("expected the author card summary to load the bio asynchronously")
	}

	page = getPage(t, app, "/authors/john-doe?l=es")
	if page.Find(`#author-john-doe [hx-get="/authors/john-doe/bio"]`).Length() != 1 {
		t.Fatal("expected the bio to be loaded asynchronously")
	}
	if client.calls != 0 {
		t.Fatal("the author page must not wait for the bio")
	}

	bio := getPage(t, app, "/authors/john-doe/bio?l=es")
	paragraphs := bio.Find("#author-bio-text p")
	if bio.Find("#author-bio-text.author-bio-text.clamp").Length() != 1 ||
		bio.Find(`.author-bio-toggle[hidden][aria-expanded="false"][aria-controls="author-bio-text"]`).Length() != 1 {
		t.Fatal("biographies must start collapsed with an expansion control")
	}
	if paragraphs.Length() != 2 || !strings.HasPrefix(paragraphs.First().Text(), "John Doe (es) is an American novelist") {
		t.Fatalf("expected the translated bio in two paragraphs, got %q", bio.Text())
	}
	if note := bio.Find("#author-bio[data-translated-from]"); note.AttrOr("data-translated-from", "") != "inglés" || !strings.Contains(note.Text(), "Traducción automática. Ver en el idioma original: inglés") {
		t.Fatalf("a translated bio must be marked as an automatic translation, got %q", bio.Text())
	}
	originalLink := bio.Find(`#author-bio a[hx-get="/authors/john-doe/bio?original=true&l=es"]`)
	if originalLink.Length() != 1 || originalLink.AttrOr("hx-target", "") != "#author-bio" {
		t.Fatal("the translation note must link to the original biography inside the card")
	}
	for _, tc := range []struct {
		path, prefix string
		translated   bool
	}{
		{originalLink.AttrOr("href", ""), "John Doe is an American novelist", false},
		{"/authors/john-doe?l=es", "John Doe (es) is an American novelist", true},
		{"/authors/john-doe?l=en", "John Doe is an American novelist", false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			page := getPage(t, app, tc.path)
			paragraphs := page.Find("#author-bio-text p")
			if page.Find(`[hx-get="/authors/john-doe/bio"]`).Length() != 0 ||
				paragraphs.Length() != 2 || !strings.HasPrefix(paragraphs.First().Text(), tc.prefix) ||
				(page.Find("#author-bio[data-translated-from]").Length() == 1) != tc.translated {
				t.Fatalf("unexpected cached biography or translation notice: %q", page.Text())
			}
			if client.calls != 1 || translator.calls != 1 {
				t.Fatal("cached biographies must not be fetched or translated again")
			}
		})
	}
	response, err := app.Test(mustGetRequest(t, "/authors/unknown/bio"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("unknown authors must return not found, got %d", response.StatusCode)
	}
}

type bioSourceCall struct {
	id, language string
	refresh      bool
}

type bioSourceResult struct {
	text  string
	err   error
	calls []bioSourceCall
}

type localizedBioSource struct {
	language string
	result   *bioSourceResult
}

func (s *localizedBioSource) ForLanguage(language string) biography.Source {
	return &localizedBioSource{language: language, result: s.result}
}
func (s *localizedBioSource) Language() string          { return s.language }
func (s *localizedBioSource) LinkKey() string           { return "catalog:" + s.language }
func (*localizedBioSource) ProfileServiceName() string  { return "Example biographies" }
func (*localizedBioSource) MetadataServiceName() string { return "Example catalog" }
func (*localizedBioSource) ProfileInputName() string    { return "catalogBioID" }
func (s *localizedBioSource) ProfileValue(author index.Author) string {
	return author.DataSourceReferences[s.LinkKey()]
}
func (*localizedBioSource) NormalizeAuthorID(value string) (string, error) {
	return strings.TrimSpace(value), nil
}
func (*localizedBioSource) CachedBioValid(string) bool { return true }
func (s *localizedBioSource) RetrieveBio(_ context.Context, id string, refresh bool) (string, string, error) {
	s.result.calls = append(s.result.calls, bioSourceCall{id, s.language, refresh})
	return s.result.text, id, s.result.err
}

type bioMetadataSource struct {
	alternativeAuthorSource
	link string
}

func (*bioMetadataSource) NormalizeAuthorID(value string) (string, error) {
	return strings.TrimPrefix(strings.TrimSpace(value), "https://example.org/authors/"), nil
}
func (s *bioMetadataSource) RetrieveAuthor(ids []string, _ []string) (datasourcemodel.Author, error) {
	return bioMetadata{sourceID: ids[0], link: s.link}, nil
}
func (*bioMetadataSource) OwnsLink(key string) bool {
	return key == "example" || strings.HasPrefix(key, "catalog:")
}

type bioMetadata struct {
	alternativeAuthorMetadata
	sourceID, link string
}

func (m bioMetadata) Links() map[string]string {
	links := map[string]string{"example": m.sourceID}
	if m.link == "" {
		return links
	}
	links["catalog:es"] = m.link
	return links
}
func (bioMetadata) OwnsLink(key string) bool {
	return (&bioMetadataSource{}).OwnsLink(key)
}

func TestLocalizedAuthorBioUsesCurrentLanguageOnAuthorPage(t *testing.T) {
	result := &bioSourceResult{}
	app, idx := newAuthorBioTestAppWithSource(t, &localizedBioSource{language: "en", result: result}, nil, &bioMetadataSource{})
	author := seedBioTestAuthor(t, idx, map[string]string{
		"catalog:en": "english-profile",
		"catalog:es": "spanish-profile",
	})
	admin, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	page := getPageWithCookie(t, app, admin, "/authors/john-doe/summary?l=es")
	form := page.Find("#update-author-form-john-doe")
	if form.Find(`input[name="sourceID"]`).Length() != 1 || form.Find("input").Length() != 1 ||
		form.Find(`input[name="catalogBioID"]`).Length() != 0 {
		t.Fatal("metadata and biographies from the same service must share a single profile input")
	}
	if page.Find(`[hx-get="/authors/john-doe/bio"]`).Length() != 1 || len(result.calls) != 0 {
		t.Fatal("a known profile must enable lazy biography retrieval without requesting the source")
	}
	if err := idx.StoreAuthorBioForLink(author.Slug, "catalog:es", author.DataSourceReferences["catalog:es"], index.AuthorBio{
		Source: author.DataSourceReferences["catalog:es"], Language: "es", Text: map[string]string{"es": "Stored Spanish biography."},
	}, true); err != nil {
		t.Fatal(err)
	}
	page = getPage(t, app, "/authors/john-doe?l=es")
	if text := strings.TrimSpace(page.Find("#author-bio-text").Text()); text != "Stored Spanish biography." {
		t.Fatalf("must render the stored biography: %q", text)
	}
	if page.Find(`[hx-get="/authors/john-doe/bio"]`).Length() != 0 ||
		page.Find("#author-bio[data-translated-from]").Length() != 0 {
		t.Fatal("native biographies must not be requested again or marked as translations")
	}
	page = getPage(t, app, "/authors/john-doe?l=fr")
	if page.Find(`[hx-get="/authors/john-doe/bio"]`).Length() != 0 || page.Find("#author-bio-text").Length() != 0 {
		t.Fatal("an unknown French profile must not display or retrieve another language's biography")
	}
	if len(result.calls) != 0 {
		t.Fatal("rendering stored or unavailable biographies must not request the source")
	}
}

func TestMetadataProfileUpdateRefreshesBiography(t *testing.T) {
	result := &bioSourceResult{text: "Updated Spanish biography."}
	source := &bioMetadataSource{link: "new-profile"}
	app, idx := newAuthorBioTestAppWithSource(t, &localizedBioSource{language: "en", result: result}, nil, source)
	author, err := idx.Author("john-doe", "es")
	if err != nil {
		t.Fatal(err)
	}
	author.DataSourceReferences = map[string]string{
		"example":     "old-author",
		"catalog:es":  "old-profile",
		"independent": "other-profile",
	}
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	if err := idx.StoreAuthorBio(author.Slug, index.AuthorBio{
		Source: author.DataSourceReferences["catalog:es"], Language: "es", Text: map[string]string{"es": "Stale biography."},
	}); err != nil {
		t.Fatal(err)
	}
	admin, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	for i, style := range []string{"", "&style=clear"} {
		response, err := putRequest(url.Values{"sourceID": {"https://example.org/authors/new-author"}}, admin, app, "/authors/john-doe?l=es"+style, t)
		if err != nil {
			t.Fatal(err)
		}
		page := parseHTMLResponse(t, response)
		if response.StatusCode != http.StatusOK || page.Find(`input[name="sourceID"]`).Length() != 1 ||
			page.Find(`input[name="catalogBioID"]`).Length() != 0 {
			t.Fatalf("unified update must return a single metadata profile input: status=%d", response.StatusCode)
		}
		stored, err := idx.Author(author.Slug, "es")
		if err != nil {
			t.Fatal(err)
		}
		if stored.DataSourceReferences["example"] != "new-author" || stored.Bio.Source != source.link || stored.Bio.Text["es"] != result.text ||
			stored.DataSourceReferences["independent"] != "other-profile" {
			t.Fatalf("metadata update must refresh and persist its biography while preserving independent links: %+v", stored)
		}
		if style == "" && strings.TrimSpace(page.Find("#author-bio-text").Text()) != result.text {
			t.Fatal("the updated author card must display the refreshed biography")
		}
		if len(result.calls) != i+1 {
			t.Fatalf("each metadata update must refresh the biography, calls=%d", len(result.calls))
		}
		if call := result.calls[i]; call != (bioSourceCall{source.link, "es", true}) {
			t.Fatalf("metadata update must refresh the new profile in the reader's language, got %+v", call)
		}
		result.text = "Refreshed Spanish biography."
	}
	response, err := postRequest(url.Values{"catalogBioID": {"other-profile"}}, admin, app, "/authors/john-doe/bio?l=es", t)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest || len(result.calls) != 2 {
		t.Fatal("the biography profile must not be editable independently of metadata")
	}
	result.err = errors.New("biography source unavailable")
	response, err = putRequest(url.Values{"sourceID": {"new-author"}}, admin, app, "/authors/john-doe?l=es", t)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("biography refresh failures must be surfaced, got %d", response.StatusCode)
	}
	result.err = nil
	source.link = ""
	for _, sourceID := range []string{"without-bio", ""} {
		beforeCalls := len(result.calls)
		response, err := putRequest(url.Values{"sourceID": {sourceID}}, admin, app, "/authors/john-doe?l=es", t)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		stored, err := idx.Author(author.Slug, "es")
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK || stored.DataSourceReferences["example"] != sourceID ||
			stored.Bio.Source != "" || len(stored.Bio.Text) != 0 ||
			stored.DataSourceReferences["catalog:es"] != "" || len(result.calls) != beforeCalls {
			t.Fatalf("missing or cleared metadata links must clear the biography without requests: %+v", stored)
		}
	}
}

func TestAuthorBioRefresh(t *testing.T) {
	t.Run("results", testAuthorBioRefreshResults)
	t.Run("permissions", testAuthorBioRefreshPermissions)
	t.Run("profile-editing", func(t *testing.T) {
		t.Run("add-profile", testAuthorBioAddProfile)
		t.Run("update-profile", testAuthorBioProfileEditing)
	})
}

func testAuthorBioRefreshResults(t *testing.T) {
	for _, state := range []string{"empty", "populated"} {
		t.Run(state, func(t *testing.T) {
			client := &bioAuthorClient{}
			app, idx := newAuthorBioTestApp(t, client, &bioTranslator{})
			author := seedBioTestAuthor(t, idx, map[string]string{"goodreads": "1.John_Doe"})
			cached := index.AuthorBio{Source: author.DataSourceReferences["goodreads"], Language: "en"}
			if state == "populated" {
				cached.Text = map[string]string{"en": "Old biography", "es": "Old translation", "fr": "Stale translation"}
			}
			if err := idx.StoreAuthorBio(author.Slug, cached); err != nil {
				t.Fatal(err)
			}
			adminCookie, err := login(app, "admin@example.com", "admin", t)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"/authors/john-doe", "/authors/john-doe/summary"} {
				page := getPageWithCookie(t, app, adminCookie, path)
				if page.Find("#author-bio-refresh").Length() != 1 {
					t.Fatalf("admins must be able to refresh a cached bio in %s", path)
				}
			}
			response, err := postRequest(url.Values{}, adminCookie, app, "/authors/john-doe/bio?l=es", t)
			if err != nil {
				t.Fatal(err)
			}
			page := parseHTMLResponse(t, response)
			if response.StatusCode != http.StatusOK || page.Find("#author-bio-text p").Length() != 2 || client.calls != 1 {
				t.Fatalf("refresh must replace a cached bio, got status %d and %d requests", response.StatusCode, client.calls)
			}
			stored, err := idx.Author("john-doe", "en")
			if err != nil {
				t.Fatal(err)
			}
			if len(stored.Bio.Text) != 2 || !strings.HasPrefix(stored.Bio.Text["en"], "John Doe is an American novelist") ||
				!strings.HasPrefix(stored.Bio.Text["es"], "John Doe (es) is an American novelist") {
				t.Fatalf("refresh must replace the biography and its translations: %+v", stored.Bio)
			}
		})
	}
}

func TestAuthorUpdateGuidance(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "wikidata-only", true: "both-sources"}[enabled], func(t *testing.T) {
			var source biography.Source
			if enabled {
				source = biblioreads.NewBioSource(&bioAuthorClient{})
			}
			app, idx := newAuthorBioTestAppWithSource(t, source, &bioTranslator{}, nil)
			seedBioTestAuthor(t, idx, nil)
			admin, err := login(app, "admin@example.com", "admin", t)
			if err != nil {
				t.Fatal(err)
			}
			for _, lang := range []string{"en", "es", "de", "fr", "ru"} {
				page := getPageWithCookie(t, app, admin, "/authors/john-doe/summary?l="+lang)
				form := page.Find("#update-author-form-john-doe")
				guidance := form.Find("p")
				if guidance.Length() != 1 || strings.Contains(guidance.Text(), "Wikidata") ||
					strings.Contains(guidance.Text(), "Goodreads") {
					t.Fatalf("%s: expected service-neutral guidance, got %q", lang, guidance.Text())
				}
				if lang != "en" && guidance.Text() == "Update the author's external profile links below. The label on each field identifies its service." {
					t.Fatalf("%s: guidance was not translated: %q", lang, guidance.Text())
				}
				if form.Find(`input[name="sourceID"][aria-label]`).Length() != 1 ||
					(form.Find(`input[name="goodreadsID"][aria-label]`).Length() == 1) != enabled {
					t.Fatalf("%s: expected labelled controls for available sources", lang)
				}
				wikidataInput := form.Find(`input[name="sourceID"]`)
				label := wikidataInput.AttrOr("aria-label", "")
				if !strings.Contains(label, "Wikidata") ||
					(lang != "en" && label == "Wikidata profile") {
					t.Fatalf("%s: expected a localized label naming the service, got %q", lang, label)
				}
				if wikidataInput.Is("[pattern], [maxlength]") {
					t.Fatalf("%s: Wikidata field must allow URLs", lang)
				}
				if enabled {
					goodreadsInput := form.Find(`input[name="goodreadsID"]`)
					goodreadsLabel := goodreadsInput.AttrOr("aria-label", "")
					if goodreadsInput.AttrOr("placeholder", "") != goodreadsLabel || !strings.Contains(goodreadsLabel, "Goodreads") ||
						(lang != "en" && goodreadsLabel == "Goodreads profile") {
						t.Fatalf("%s: each profile field must use its service-provided label", lang)
					}
				}
			}
		})
	}
}

func testAuthorBioRefreshPermissions(t *testing.T) {
	client := &bioAuthorClient{}
	app, idx := newAuthorBioTestApp(t, client, &bioTranslator{})

	seedBioTestAuthor(t, idx, map[string]string{"goodreads": "1.John_Doe"})
	getBody(t, app, "/authors/john-doe/bio?l=es")
	getBody(t, app, "/authors/john-doe/bio?l=en")

	for _, path := range []string{"/authors/john-doe", "/authors/john-doe/summary"} {
		if getPage(t, app, path).Find("#author-bio-refresh").Length() != 0 {
			t.Errorf("anonymous visitors must not see a bio refresh link in %s", path)
		}
	}
	response, err := postRequest(url.Values{}, &http.Cookie{Name: "anonymous"}, app, "/authors/john-doe/bio", t)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode == http.StatusOK || client.calls != 1 {
		t.Fatalf("anonymous users must not refresh biographies, got status %d and %d requests", response.StatusCode, client.calls)
	}

	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	addRegularUser(t, app, adminCookie)
	regularCookie, err := login(app, "regular@example.com", "regular", t)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/authors/john-doe", "/authors/john-doe/summary", "/authors/john-doe/bio"} {
		page := getPageWithCookie(t, app, regularCookie, path)
		if page.Find("#author-bio").Length() != 1 || page.Find("#author-bio-refresh").Length() != 0 {
			t.Errorf("regular users must see the bio but not the refresh link in %s", path)
		}
	}
	response, err = postRequest(url.Values{}, regularCookie, app, "/authors/john-doe/bio", t)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden || client.calls != 1 {
		t.Fatalf("regular users must not refresh biographies, got status %d and %d requests", response.StatusCode, client.calls)
	}
}

func testAuthorBioProfileEditing(t *testing.T) {
	client := &bioAuthorClient{}
	app, idx := newAuthorBioTestApp(t, client, &bioTranslator{})
	seedBioTestAuthor(t, idx, map[string]string{"goodreads": "1.John_Doe"})
	getBody(t, app, "/authors/john-doe/bio?l=es")
	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	page := getPageWithCookie(t, app, adminCookie, "/authors/john-doe")
	if page.Find(`#author-john-doe #author-bio-refresh[hx-post="/authors/john-doe/bio"]`).Length() != 1 {
		t.Fatal("admins must see a form to update the biography")
	}
	if page.Find(`#update-author-form-john-doe.collapse form input[name="goodreadsID"]`).AttrOr("value", "") != "https://www.goodreads.com/author/show/1.John_Doe" ||
		page.Find(`#update-author-form-john-doe input[name="sourceID"]`).Length() != 1 ||
		page.Find(`#update-bio-form-john-doe`).Length() != 0 {
		t.Fatal("the shared Wikidata collapsible must contain both fields and prefill the Goodreads profile URL")
	}
	if page.Find(`#author-bio-refresh[hx-indicator="#author-bio"]`).Length() != 1 || page.Find("#author-bio .author-bio-loading .placeholder").Length() != 3 {
		t.Fatal("refreshing a biography must activate its loading skeleton")
	}

	response, err := postRequest(url.Values{"goodreadsID": {"https://www.goodreads.com/author/show/879.Plato/"}}, adminCookie, app, "/authors/john-doe/bio?l=es", t)
	if err != nil {
		t.Fatal(err)
	}
	page = parseHTMLResponse(t, response)
	stored, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || stored.DataSourceReferences["goodreads"] != "879.Plato" || stored.Bio.Source != "879.Plato" ||
		page.Find("#author-john-doe").Length() != 1 || page.Find(`a[href="https://www.goodreads.com/author/show/879.Plato"]`).Length() != 1 ||
		page.Find(`input[name="goodreadsID"]`).AttrOr("value", "") != "https://www.goodreads.com/author/show/879.Plato" {
		t.Fatalf("updating the Goodreads ID must refresh the whole card and persist the new source, got %+v", stored.Bio)
	}
	for _, invalid := range []string{"0", "https://example.com/author/show/879"} {
		response, err := postRequest(url.Values{"goodreadsID": {invalid}}, adminCookie, app, "/authors/john-doe/bio", t)
		if err != nil {
			t.Fatal(err)
		}
		page := parseHTMLResponse(t, response)
		if response.StatusCode != http.StatusBadRequest || client.calls != 2 {
			t.Errorf("invalid ID %q must be rejected before lookup, got %d", invalid, response.StatusCode)
		}
		form := page.Find("#author-john-doe #update-author-form-john-doe.show")
		input := form.Find(`input[name="goodreadsID"][aria-invalid="true"]`)
		if input.Length() != 1 || input.AttrOr("value", "") != invalid ||
			form.Find(`#profile-error-john-doe[role="alert"]`).Length() != 1 {
			t.Errorf("invalid ID %q must preserve the author card and input with a visible validation error", invalid)
		}
	}
	response, err = postRequest(url.Values{"goodreadsID": {"  "}}, adminCookie, app, "/authors/john-doe/bio?l=es", t)
	if err != nil {
		t.Fatal(err)
	}
	page = parseHTMLResponse(t, response)
	stored, err = idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || stored.DataSourceReferences["goodreads"] != "" || stored.Bio.Source != "" ||
		len(stored.Bio.Text) != 0 || client.calls != 2 || page.Find(`a[href*="goodreads.com/author/"]`).Length() != 0 ||
		page.Find(`input[name="goodreadsID"]`).AttrOr("value", "") != "" {
		t.Fatalf("blank Goodreads ID must clear link and biography without lookup: status=%d author=%+v calls=%d", response.StatusCode, stored, client.calls)
	}
}
