package webserver_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/precisiondate"
	"github.com/svera/coreander/v5/internal/translation"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
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

func (t *bioTranslator) Translate(_ context.Context, text, from, to string, _ translation.Gender) (string, string, error) {
	t.calls++
	return strings.ReplaceAll(text, "John Doe", "John Doe ("+to+")"), from, nil
}

// lazyBioIndex lets the provider be configured before the index is created.
type lazyBioIndex struct{ idx *index.BleveIndexer }

func (l *lazyBioIndex) StoreAuthorBio(slug string, bio index.AuthorBio) error {
	return l.idx.StoreAuthorBio(slug, bio)
}

func (l *lazyBioIndex) StoreAuthorBioForLink(slug, linkKey, expectedLink string, bio index.AuthorBio, merge bool) error {
	return l.idx.StoreAuthorBioForLink(slug, linkKey, expectedLink, bio, merge)
}

func (l *lazyBioIndex) ReplaceAuthorBio(slug string, bio index.AuthorBio) error {
	return l.idx.ReplaceAuthorBio(slug, bio)
}

func (l *lazyBioIndex) StoreCanonicalAuthorBio(slug, linkKey, previousLink string, bio index.AuthorBio) error {
	return l.idx.StoreCanonicalAuthorBio(slug, linkKey, previousLink, bio)
}

func newAuthorBioTestApp(t *testing.T, client *bioAuthorClient, translator biblioreads.Translator) (*fiber.App, *index.BleveIndexer) {
	t.Helper()
	bioIndex := &lazyBioIndex{}
	cfg := defaultTestConfig()
	cfg.AuthorBioProvider = biblioreads.NewBioProvider(client, translator, bioIndex)
	app, idx := bootstrapAppWithIndexer(infrastructure.Connect(":memory:", 250), &infrastructure.NoEmail{}, loadDirInMemoryFs("testdata/library"), cfg)
	bioIndex.idx = idx
	return app, idx
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

type alternateProfileInput struct{ *biblioreads.BioProvider }

func (alternateProfileInput) ProfileInputName() string   { return "otherProfileID" }
func (alternateProfileInput) ProfileServiceName() string { return "Other service" }

func TestAuthorBioProfileInputFromProvider(t *testing.T) {
	client, bioIndex := &bioAuthorClient{}, &lazyBioIndex{}
	cfg := defaultTestConfig()
	cfg.AuthorBioProvider = alternateProfileInput{biblioreads.NewBioProvider(client, nil, bioIndex)}
	app, idx := bootstrapAppWithIndexer(infrastructure.Connect(":memory:", 250), &infrastructure.NoEmail{}, loadDirInMemoryFs("testdata/library"), cfg)
	bioIndex.idx = idx
	author, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	author.RetrievedOn = time.Now()
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	admin, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	response, err := getRequest(admin, app, "/authors/john-doe/summary", t)
	if err != nil {
		t.Fatal(err)
	}
	page := parseHTMLResponse(t, response)
	if page.Find(`input[name="otherProfileID"][aria-label="Other service profile"]`).Length() != 1 ||
		page.Find(`input[name="goodreadsID"]`).Length() != 0 {
		t.Fatal("profile input must use the biography provider's name and label")
	}
	for _, tc := range []struct {
		input      string
		wantStatus int
	}{
		{"invalid", http.StatusBadRequest},
		{"879", http.StatusOK},
	} {
		response, err = postRequest(url.Values{"otherProfileID": {tc.input}}, admin, app, "/authors/john-doe/bio", t)
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
	if author.DataSourceLinks["goodreads"] != "879" || client.calls != 1 {
		t.Fatal("valid provider-specific input must update the biography")
	}
}

func TestAuthorDetailCardLayout(t *testing.T) {
	app, idx := newAuthorBioTestApp(t, &bioAuthorClient{}, nil)
	author, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	author.DataSourceID = "Q1234"
	author.Description = map[string]string{"en": "American novelist"}
	author.RetrievedOn = time.Now()
	author.DateOfBirth = precisiondate.NewPrecisionDate("+1920-01-02T00:00:00Z", precisiondate.PrecisionDay)
	author.DateOfDeath = precisiondate.NewPrecisionDate("+1992-04-06T00:00:00Z", precisiondate.PrecisionDay)
	author.DataSourceLinks = map[string]string{
		"goodreads": "1.John_Doe",
		"website":   "https://example.com",
	}
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	if err := idx.StoreAuthorBio(author.Slug, index.AuthorBio{
		Source: author.DataSourceLinks["goodreads"], Language: "en",
		Text: map[string]string{"en": "John Doe wrote many books."},
	}); err != nil {
		t.Fatal(err)
	}
	admin, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/authors/john-doe?l=en", "/authors/john-doe/summary?l=en"} {
		t.Run(path, func(t *testing.T) {
			response, err := getRequest(admin, app, path, t)
			if err != nil {
				t.Fatal(err)
			}
			mustReturnStatus(response, http.StatusOK, t)
			page := parseHTMLResponse(t, response)
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

	author, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	author.DataSourceLinks = map[string]string{"goodreads": "1.John_Doe"}
	author.RetrievedOn = time.Now()
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
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
	original := getPage(t, app, originalLink.AttrOr("href", ""))
	if !strings.HasPrefix(original.Find("#author-bio-text p").First().Text(), "John Doe is an American novelist") || original.Find("#author-bio[data-translated-from]").Length() != 0 {
		t.Fatal("the original biography must be shown without a translation notice")
	}
	if client.calls != 1 || translator.calls != 1 {
		t.Fatal("showing the original biography must not fetch or translate it again")
	}

	page = getPage(t, app, "/authors/john-doe?l=es")
	if page.Find(`[hx-get="/authors/john-doe/bio"]`).Length() != 0 || page.Find("#author-john-doe #author-bio-text p").Length() != 2 {
		t.Fatal("a stored bio must be rendered with the author page")
	}
	if page.Find("#author-bio[data-translated-from]").Length() != 1 {
		t.Fatal("a stored translated bio must be marked as an automatic translation")
	}
	page = getPage(t, app, "/authors/john-doe?l=en")
	if page.Find(`[hx-get="/authors/john-doe/bio"]`).Length() != 0 || !strings.HasPrefix(page.Find("#author-john-doe #author-bio p").First().Text(), "John Doe is an American") {
		t.Fatal("the original bio must be rendered without translating it")
	}
	if page.Find("#author-bio[data-translated-from]").Length() != 0 {
		t.Fatal("an original bio must not be marked as translated")
	}
	formatted := index.AuthorBio{Source: author.DataSourceLinks["goodreads"], Language: "en", Text: map[string]string{
		"en": `<b onclick="bad()">John Doe</b> wrote <i>books</i>.<script>alert(1)</script>`,
	}}
	if err := idx.ReplaceAuthorBio(author.Slug, formatted); err != nil {
		t.Fatal(err)
	}
	formattedPage := getPage(t, app, "/authors/john-doe/bio?original=true&l=es")
	if formattedPage.Find("#author-bio-text b").Text() != "John Doe" || formattedPage.Find("#author-bio-text i").Text() != "books" || formattedPage.Find("#author-bio-text script, #author-bio-text [onclick]").Length() != 0 {
		t.Fatal("biographies must render safe bold and italic markup only")
	}
	if client.calls != 1 || translator.calls != 1 {
		t.Errorf("expected one Goodreads request and one translation, got %d and %d", client.calls, translator.calls)
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

func TestAuthorBioRefreshEmpty(t *testing.T) {
	client := &bioAuthorClient{}
	app, idx := newAuthorBioTestApp(t, client, &bioTranslator{})
	author, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	author.DataSourceLinks = map[string]string{"goodreads": "1.John_Doe"}
	author.RetrievedOn = time.Now()
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	if err := idx.StoreAuthorBio(author.Slug, index.AuthorBio{Source: author.DataSourceLinks["goodreads"]}); err != nil {
		t.Fatal(err)
	}
	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/authors/john-doe", "/authors/john-doe/summary"} {
		response, err := getRequest(adminCookie, app, path, t)
		if err != nil {
			t.Fatal(err)
		}
		page := parseHTMLResponse(t, response)
		if page.Find("#author-bio-refresh").Length() != 1 {
			t.Fatalf("admins must be able to refresh a cached empty bio in %s", path)
		}
		if getPage(t, app, path).Find("#author-bio-refresh").Length() != 0 {
			t.Fatalf("anonymous visitors must not see an empty bio refresh link in %s", path)
		}
	}
	response, err := postRequest(url.Values{}, adminCookie, app, "/authors/john-doe/bio?l=es", t)
	if err != nil {
		t.Fatal(err)
	}
	page := parseHTMLResponse(t, response)
	if response.StatusCode != http.StatusOK || page.Find("#author-bio-text p").Length() != 2 || client.calls != 1 {
		t.Fatalf("refresh must recover a cached empty bio, got status %d and %d requests", response.StatusCode, client.calls)
	}
}

func TestAuthorBioAddGoodreadsID(t *testing.T) {
	client := &bioAuthorClient{}
	app, idx := newAuthorBioTestApp(t, client, &bioTranslator{})
	author, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	author.RetrievedOn = time.Now()
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	response, err := getRequest(adminCookie, app, "/authors/john-doe/summary", t)
	if err != nil {
		t.Fatal(err)
	}
	page := parseHTMLResponse(t, response)
	if page.Find(`#author-bio-refresh input[name="goodreadsID"]`).Length() != 1 || client.calls != 0 {
		t.Fatal("admins must be able to add a Goodreads ID without an existing link")
	}
	response, err = postRequest(url.Values{"goodreadsID": {"879"}}, adminCookie, app, "/authors/john-doe/bio", t)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	stored, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || stored.DataSourceLinks["goodreads"] != "879" || stored.Bio.Text["en"] == "" {
		t.Fatal("adding a Goodreads ID must store its link and biography")
	}
}

func TestAuthorUpdateGuidance(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "wikidata-only", true: "both-sources"}[enabled], func(t *testing.T) {
			cfg := defaultTestConfig()
			bioIndex := &lazyBioIndex{}
			if enabled {
				cfg.AuthorBioProvider = biblioreads.NewBioProvider(&bioAuthorClient{}, &bioTranslator{}, bioIndex)
			}
			app, idx := bootstrapAppWithIndexer(infrastructure.Connect(":memory:", 250), &infrastructure.NoEmail{}, loadDirInMemoryFs("testdata/library"), cfg)
			bioIndex.idx = idx
			author, err := idx.Author("john-doe", "en")
			if err != nil {
				t.Fatal(err)
			}
			author.RetrievedOn = time.Now()
			if err := idx.IndexAuthor(author); err != nil {
				t.Fatal(err)
			}
			admin, err := login(app, "admin@example.com", "admin", t)
			if err != nil {
				t.Fatal(err)
			}
			for _, lang := range []string{"en", "es", "de", "fr", "ru"} {
				response, err := getRequest(admin, app, "/authors/john-doe/summary?l="+lang, t)
				if err != nil {
					t.Fatal(err)
				}
				page := parseHTMLResponse(t, response)
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

func TestAuthorBioRefresh(t *testing.T) {
	client := &bioAuthorClient{}
	app, idx := newAuthorBioTestApp(t, client, &bioTranslator{})

	author, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	author.DataSourceLinks = map[string]string{"goodreads": "1.John_Doe"}
	author.RetrievedOn = time.Now()
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	getBody(t, app, "/authors/john-doe/bio?l=es")
	getBody(t, app, "/authors/john-doe/bio?l=en")

	if getPage(t, app, "/authors/john-doe").Find("#author-bio-refresh").Length() != 0 {
		t.Error("only admins can refresh biographies")
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
		response, err := getRequest(regularCookie, app, path, t)
		if err != nil {
			t.Fatal(err)
		}
		page := parseHTMLResponse(t, response)
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

	response, err = getRequest(adminCookie, app, "/authors/john-doe", t)
	if err != nil {
		t.Fatal(err)
	}
	page := parseHTMLResponse(t, response)
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

	client.desc = "John Doe is an American novelist and sailor who now lives in Boston with his family."
	response, err = postRequest(url.Values{}, adminCookie, app, "/authors/john-doe/bio?l=es", t)
	if err != nil {
		t.Fatal(err)
	}
	page = parseHTMLResponse(t, response)
	if response.StatusCode != http.StatusOK || client.calls != 2 || !strings.Contains(page.Find("#author-bio").Text(), "John Doe (es) is an American novelist and sailor") {
		t.Fatalf("expected the refreshed bio, got status %d, %d requests and %q", response.StatusCode, client.calls, page.Text())
	}

	stored, err := idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Bio.Text) != 2 || !strings.HasPrefix(stored.Bio.Text["en"], "John Doe is an American novelist and sailor") {
		t.Errorf("the refreshed bio must replace the stored one and its translations, got %+v", stored.Bio)
	}
	response, err = postRequest(url.Values{"goodreadsID": {"https://www.goodreads.com/author/show/879.Plato/"}}, adminCookie, app, "/authors/john-doe/bio?l=es", t)
	if err != nil {
		t.Fatal(err)
	}
	page = parseHTMLResponse(t, response)
	stored, err = idx.Author("john-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || stored.DataSourceLinks["goodreads"] != "879.Plato" || stored.Bio.Source != "879.Plato" ||
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
		if response.StatusCode != http.StatusBadRequest || client.calls != 3 {
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
	if response.StatusCode != http.StatusOK || stored.DataSourceLinks["goodreads"] != "" || stored.Bio.Source != "" ||
		len(stored.Bio.Text) != 0 || client.calls != 3 || page.Find(`a[href*="goodreads.com/author/"]`).Length() != 0 ||
		page.Find(`input[name="goodreadsID"]`).AttrOr("value", "") != "" {
		t.Fatalf("blank Goodreads ID must clear link and biography without lookup: status=%d author=%+v calls=%d", response.StatusCode, stored, client.calls)
	}
}
