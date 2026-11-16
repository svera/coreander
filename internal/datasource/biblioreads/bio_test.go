package biblioreads_test

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/translation"
)

const (
	englishBio = `Stephen Edwin King was born the second son of Donald and Nellie Ruth Pillsbury King.<br /><br />After his father left them when Stephen was two, he and his older brother, David, were raised by his mother. Parts of his childhood were spent in 
  <a href="https://example.com">Fort Wayne</a>
, Indiana &amp; Stratford.`
	spanishBio = `Arturo Pérez-Reverte es un escritor y periodista español, miembro de la Real Academia Española. Fue corresponsal de guerra durante más de veinte años y es autor de numerosas novelas.`
)

type fakeAuthorClient struct {
	desc      string
	err       error
	calls     []string
	responses map[string]string
}

func (f *fakeAuthorClient) Author(_ context.Context, id string) (biblioreads.AuthorResponse, error) {
	f.calls = append(f.calls, id)
	if f.responses != nil {
		return biblioreads.AuthorResponse{Desc: f.responses[id]}, f.err
	}
	return biblioreads.AuthorResponse{Desc: f.desc}, f.err
}

func (f *fakeAuthorClient) RefreshAuthor(ctx context.Context, id string) (biblioreads.AuthorResponse, error) {
	return f.Author(ctx, id)
}

type translateCall struct{ text, from, to string }

type fakeTranslator struct {
	detected string
	err      error
	calls    []translateCall
}

func (f *fakeTranslator) Translate(_ context.Context, text, from, to string) (string, string, error) {
	f.calls = append(f.calls, translateCall{text, from, to})
	if f.err != nil {
		return "", "", f.err
	}
	if from == translation.AutoDetect {
		from = f.detected
	}
	return "[" + to + "] " + text, from, nil
}

type fakeBioIndex struct {
	mu             sync.Mutex
	stored         []index.AuthorBio
	replaced       []index.AuthorBio
	previousSource string
}

func (f *fakeBioIndex) StoreCanonicalAuthorBio(slug, previousSource string, bio index.AuthorBio) error {
	f.previousSource = previousSource
	return f.ReplaceAuthorBio(slug, bio)
}

func (f *fakeBioIndex) StoreAuthorBioForSource(slug, source string, bio index.AuthorBio, merge bool) error {
	if merge {
		return f.StoreAuthorBio(slug, bio)
	}
	return f.ReplaceAuthorBio(slug, bio)
}

func (f *fakeBioIndex) ReplaceAuthorBio(_ string, bio index.AuthorBio) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replaced = append(f.replaced, bio)
	return nil
}

func (f *fakeBioIndex) StoreAuthorBio(_ string, bio index.AuthorBio) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stored = append(f.stored, bio)
	return nil
}

func goodreadsAuthor(bio index.AuthorBio) index.Author {
	return index.Author{
		Slug: "stephen-king", Name: "Stephen King",
		DataSourceLinks: map[string]string{"goodreads": "3389.Stephen_King"},
		Bio:             bio,
	}
}

func TestBioRejectsUnsupportedInstance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode(biblioreads.AuthorResponse{
			Version: "BiblioReads v4.5.0", Status: "Received", Desc: englishBio,
		}); err != nil {
			t.Errorf("encode author response: %v", err)
		}
	}))
	defer server.Close()
	idx := &fakeBioIndex{}
	provider := biblioreads.NewBioProvider(newTestClient(biblioreads.WithInstances([]string{server.URL})), nil, idx)
	author := goodreadsAuthor(index.AuthorBio{})
	for _, action := range []struct {
		name string
		call func() (string, string, error)
	}{
		{"initial", func() (string, string, error) { return provider.Bio(context.Background(), author, "en") }},
		{"refresh", func() (string, string, error) { return provider.RefreshBio(context.Background(), author, "en") }},
		{"update", func() (string, string, error) {
			return provider.UpdateBio(context.Background(), author, "3389.Stephen_King", "en")
		}},
	} {
		t.Run(action.name, func(t *testing.T) {
			bio, _, err := action.call()
			if bio != "" || !errors.Is(err, biblioreads.ErrUnavailable) || len(idx.stored) != 0 || len(idx.replaced) != 0 {
				t.Fatalf("unsupported instance must not store a biography: %q, %v, %+v", bio, err, idx)
			}
		})
	}
}

func TestBioRefreshBypassesResponseCache(t *testing.T) {
	const variantID = "18647721.Plat_n"
	const primaryID = "879.Plato"
	const variantNote = `Librarian Note: This is an alternate name variant profile. See primary profile: <a href="/author/show/879.Plato">Plato</a>`
	for _, action := range []string{"refresh", "update"} {
		for _, redirect := range []bool{false, true} {
			name := action
			if redirect {
				name += "-primary-profile"
			}
			t.Run(name, func(t *testing.T) {
				var fresh atomic.Bool
				var hits atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					hits.Add(1)
					var payload struct {
						QueryURL string `json:"queryURL"`
					}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Errorf("decode author request: %v", err)
						http.Error(w, "invalid request", http.StatusBadRequest)
						return
					}
					desc := englishBio
					if strings.HasSuffix(payload.QueryURL, variantID) {
						desc = variantNote
					} else if fresh.Load() {
						desc += " This biography has been updated."
					}
					if err := json.NewEncoder(w).Encode(biblioreads.AuthorResponse{Version: "BiblioReads v4.5.1", Status: "Received", Desc: desc}); err != nil {
						t.Errorf("encode author response: %v", err)
					}
				}))
				defer server.Close()

				client := newTestClient(biblioreads.WithInstances([]string{server.URL}))
				idx := &fakeBioIndex{}
				provider := biblioreads.NewBioProvider(client, nil, idx)
				id := primaryID
				profiles := []string{primaryID}
				if redirect {
					id = variantID
					profiles = []string{variantID, primaryID}
				}
				author := index.Author{Slug: "plato", DataSourceLinks: map[string]string{"goodreads": id}}
				old, _, err := provider.Bio(context.Background(), author, "en")
				if err != nil || strings.Contains(old, "updated") {
					t.Fatalf("initial biography: %q, %v", old, err)
				}
				if hits.Load() != int32(len(profiles)) {
					t.Fatalf("expected initial profile requests, got %d", hits.Load())
				}
				fresh.Store(true)
				var bio string
				if action == "refresh" {
					bio, _, err = provider.RefreshBio(context.Background(), author, "en")
				} else {
					bio, _, err = provider.UpdateBio(context.Background(), author, id, "en")
				}
				if err != nil || !strings.Contains(bio, "This biography has been updated.") {
					t.Fatalf("manual update must retrieve the new biography: %q, %v", bio, err)
				}
				if hits.Load() != int32(2*len(profiles)) {
					t.Fatalf("manual update must refetch every profile, got %d requests", hits.Load())
				}
				if len(idx.replaced) == 0 || !strings.Contains(idx.replaced[len(idx.replaced)-1].Text["en"], "updated") {
					t.Fatalf("updated biography must be stored: %+v", idx.replaced)
				}
				for _, profile := range profiles {
					response, err := client.Author(context.Background(), profile)
					if err != nil {
						t.Fatal(err)
					}
					if profile == primaryID && !strings.Contains(response.Desc, "updated") {
						t.Fatalf("refreshed response must replace the cache: %q", response.Desc)
					}
				}
				if hits.Load() != int32(2*len(profiles)) {
					t.Fatalf("ordinary lookups must use refreshed cache entries, got %d requests", hits.Load())
				}
			})
		}
	}
}

func TestFailedAuthorRefreshPreservesCachedResponse(t *testing.T) {
	var fail atomic.Bool
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if fail.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := json.NewEncoder(w).Encode(biblioreads.AuthorResponse{Version: "BiblioReads v4.5.1", Status: "Received", Desc: englishBio}); err != nil {
			t.Errorf("encode author response: %v", err)
		}
	}))
	defer server.Close()
	client := newTestClient(biblioreads.WithInstances([]string{server.URL}))
	if _, err := client.Author(context.Background(), "879.Plato"); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	if _, err := client.RefreshAuthor(context.Background(), "879.Plato"); err == nil {
		t.Fatal("a failed refresh must return an error")
	}
	response, err := client.Author(context.Background(), "879.Plato")
	if err != nil || response.Desc != englishBio || hits.Load() != 2 {
		t.Fatalf("a failed refresh must leave the previous cached response intact: %+v, %v, hits %d", response, err, hits.Load())
	}
}

func TestUpdateBio(t *testing.T) {
	for _, previous := range []string{"", "3389.Stephen_King"} {
		t.Run(previous, func(t *testing.T) {
			client := &fakeAuthorClient{responses: map[string]string{
				"18647721.Plat_n": `Librarian Note: This is an alternate name variant profile. See primary profile: <a href="/author/show/879.Plato">Plato</a>`,
				"879.Plato":       englishBio,
			}}
			idx := &fakeBioIndex{}
			provider := biblioreads.NewBioProvider(client, &fakeTranslator{}, idx)
			author := index.Author{Slug: "platon", DataSourceLinks: map[string]string{"goodreads": previous}}
			text, _, err := provider.UpdateBio(context.Background(), author, "18647721.Plat_n", "es")
			if err != nil || text == "" || idx.previousSource != previous || len(idx.replaced) != 1 || idx.replaced[0].Source != "879.Plato" {
				t.Fatalf("editing a Goodreads ID must resolve and save its primary profile: %q, %v, %+v", text, err, idx)
			}
			if author.DataSourceLinks["goodreads"] != previous {
				t.Fatal("updating must not mutate the caller's author")
			}
		})
	}
	client := &fakeAuthorClient{err: errors.New("unavailable")}
	idx := &fakeBioIndex{}
	provider := biblioreads.NewBioProvider(client, nil, idx)
	if _, _, err := provider.UpdateBio(context.Background(), goodreadsAuthor(index.AuthorBio{}), "879", "es"); err == nil || len(idx.replaced) != 0 || len(idx.stored) != 0 {
		t.Fatal("failed fetches must preserve the existing bio and link")
	}
}

func TestBioPreservesEmphasis(t *testing.T) {
	idx := &fakeBioIndex{}
	client := &fakeAuthorClient{desc: `<b onclick="bad()">Isaac Asimov</b> wrote <i>Foundation</i> and <strong>many</strong> <em>books</em>.<script>alert(1)</script>`}
	provider := biblioreads.NewBioProvider(client, &fakeTranslator{detected: "en"}, idx)
	text, _, err := provider.Bio(context.Background(), goodreadsAuthor(index.AuthorBio{}), "es")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"<b>[es] Isaac Asimov</b>", "<i>[es] Foundation</i>", "<strong>[es] many</strong>", "<em>[es] books</em>"} {
		if !strings.Contains(text, fragment) {
			t.Errorf("translation lost emphasis %q: %s", fragment, text)
		}
	}
	if strings.Contains(text, "onclick") || strings.Contains(text, "script") || strings.Contains(text, "alert") {
		t.Fatalf("unsafe markup must be removed: %s", text)
	}
	if len(idx.stored) != 1 || !strings.Contains(idx.stored[0].Text["en"], "<i>Foundation</i>") {
		t.Fatal("original biography must retain emphasis")
	}
}

func TestBioPrimaryProfile(t *testing.T) {
	const note = `<i>Librarian Note: This is an alternate name variant profile. See primary profile: <b><a href="https://www.goodreads.com/author/show/879.Plato">Plato</a></b>. There is more than one author by this name; see <a href="/author/show/18648109">Platon</a>.</i>`
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "cached-note"}[cached], func(t *testing.T) {
			client := &fakeAuthorClient{responses: map[string]string{
				"18647721.Plat_n": note,
				"879.Plato":       englishBio,
			}}
			idx := &fakeBioIndex{}
			provider := biblioreads.NewBioProvider(client, &fakeTranslator{}, idx)
			author := index.Author{Slug: "platon", DataSourceLinks: map[string]string{"goodreads": "18647721.Plat_n"}}
			if cached {
				author.Bio = index.AuthorBio{Source: author.DataSourceLinks["goodreads"], Language: "en", Text: map[string]string{
					"en": "Librarian Note: This is an alternate name variant profile. See primary profile: Plato.",
					"es": "Previously translated variant note",
				}}
			}
			if _, _, pending := provider.StoredBio(author, "es"); !pending {
				t.Fatal("variant notes must be resolved even when already cached")
			}
			text, from, err := provider.Bio(context.Background(), author, "es")
			if err != nil || from != "en" || !strings.HasPrefix(text, "[es] Stephen Edwin King") {
				t.Fatalf("expected the primary profile bio, got %q, %q, %v", text, from, err)
			}
			if !slices.Equal(client.calls, []string{"18647721.Plat_n", "879.Plato"}) {
				t.Fatalf("must follow only the primary profile link, got %v", client.calls)
			}
			if idx.previousSource != author.DataSourceLinks["goodreads"] || len(idx.replaced) != 1 || idx.replaced[0].Source != "879.Plato" || len(idx.stored) != 0 {
				t.Fatalf("canonical link and bio must replace the variant: %+v", idx)
			}
		})
	}
}

func TestBioPrimaryProfileGuards(t *testing.T) {
	note := func(target string) string {
		return `Librarian Note: This is an alternate name variant profile. See primary profile: <a href="` + target + `">Primary</a>`
	}
	for _, tt := range []struct {
		name      string
		desc      string
		wantError bool
	}{
		{"ordinary-link", `A writer influenced by <a href="/author/show/879.Plato">Plato</a>.`, false},
		{"disambiguation", `There is more than one author by this name. See <a href="/author/show/879.Plato">Plato</a>.`, false},
		{"external", note("https://example.com/author/show/879.Plato"), true},
		{"invalid-path", note("https://www.goodreads.com/book/show/879"), true},
		{"missing-link", "Librarian Note: This is an alternate name variant profile. See primary profile: Plato.", true},
		{"self-loop", note("/author/show/3389.Another_Slug"), true},
		{"cycle", note("/author/show/879.Plato"), true},
		{"redirect-limit", note("/author/show/1"), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeAuthorClient{responses: map[string]string{
				"3389.Stephen_King": tt.desc,
				"879.Plato":         note("/author/show/3389.Stephen_King"),
				"1":                 note("/author/show/2"),
				"2":                 note("/author/show/3"),
				"3":                 note("/author/show/4"),
				"4":                 note("/author/show/5"),
			}}
			idx := &fakeBioIndex{}
			provider := biblioreads.NewBioProvider(client, nil, idx)
			_, _, err := provider.Bio(context.Background(), goodreadsAuthor(index.AuthorBio{}), "en")
			if (err != nil) != tt.wantError {
				t.Fatalf("unexpected resolution error: %v", err)
			}
			if len(idx.replaced) != 0 || (tt.wantError && len(idx.stored) != 0) {
				t.Fatal("failed resolutions must not persist a variant bio")
			}
			if !tt.wantError && len(client.calls) != 1 {
				t.Fatal("ordinary author links must not be followed")
			}
		})
	}
}

func TestBioRetrievesAndTranslatesBio(t *testing.T) {
	client := &fakeAuthorClient{desc: englishBio}
	translator := &fakeTranslator{}
	idx := &fakeBioIndex{}
	provider := biblioreads.NewBioProvider(client, translator, idx)

	author := goodreadsAuthor(index.AuthorBio{})
	if _, _, pending := provider.StoredBio(author, "es"); !pending {
		t.Fatal("a bio that has not been retrieved yet must be pending")
	}
	bio, translatedFrom, err := provider.Bio(context.Background(), author, "es")
	if err != nil {
		t.Fatal(err)
	}
	if translatedFrom != "en" {
		t.Errorf("a translated bio must report its original language, got %q", translatedFrom)
	}
	original := "Stephen Edwin King was born the second son of Donald and Nellie Ruth Pillsbury King.\n\n" +
		"After his father left them when Stephen was two, he and his older brother, David, were raised by his mother. Parts of his childhood were spent in Fort Wayne, Indiana &amp; Stratford."
	if len(client.calls) != 1 || client.calls[0] != "3389.Stephen_King" {
		t.Fatalf("expected one Goodreads author request, got %q", client.calls)
	}
	if len(translator.calls) != 1 || translator.calls[0] != (translateCall{html.UnescapeString(original), "en", "es"}) {
		t.Fatalf("expected the plain text bio to be translated from its detected language, got %+v", translator.calls)
	}
	if bio != "[es] "+original {
		t.Fatalf("unexpected bio %q", bio)
	}
	want := index.AuthorBio{Source: "3389.Stephen_King", Language: "en", Text: map[string]string{"en": original, "es": "[es] " + original}}
	if len(idx.stored) != 1 || !equalBio(idx.stored[0], want) {
		t.Fatalf("expected original and translation to be stored, got %+v", idx.stored)
	}

	author.Bio = want
	for _, language := range []string{"en", "es"} {
		wantFrom := map[string]string{"en": "", "es": "en"}[language]
		if bio, translatedFrom, pending := provider.StoredBio(author, language); pending || bio != want.Text[language] || translatedFrom != wantFrom {
			t.Errorf("stored %s bio must be returned without lookups, got %q, %q, %t", language, bio, translatedFrom, pending)
		}
	}
	if _, _, pending := provider.StoredBio(author, "fr"); !pending {
		t.Fatal("a missing translation must be pending")
	}
	if bio, _, err := provider.Bio(context.Background(), author, "fr"); err != nil || bio != "[fr] "+original {
		t.Fatalf("unexpected French bio %q, %v", bio, err)
	}
	if len(client.calls) != 1 {
		t.Error("translating a stored bio must not request Goodreads again")
	}
	if last := idx.stored[len(idx.stored)-1]; last.Language != "en" || last.Text["fr"] != "[fr] "+original {
		t.Errorf("expected the French translation to be stored, got %+v", last)
	}
}

func TestBioInReaderLanguageIsNotTranslated(t *testing.T) {
	translator := &fakeTranslator{}
	idx := &fakeBioIndex{}
	provider := biblioreads.NewBioProvider(&fakeAuthorClient{desc: spanishBio}, translator, idx)

	bio, translatedFrom, err := provider.Bio(context.Background(), goodreadsAuthor(index.AuthorBio{}), "es")
	if err != nil || bio != spanishBio || translatedFrom != "" {
		t.Fatalf("unexpected bio %q, %q, %v", bio, translatedFrom, err)
	}
	if len(translator.calls) != 0 {
		t.Errorf("a bio in the reader language must not be translated, got %+v", translator.calls)
	}
	if len(idx.stored) != 1 || idx.stored[0].Language != "es" || idx.stored[0].Text["es"] != spanishBio {
		t.Errorf("expected the Spanish bio to be stored, got %+v", idx.stored)
	}
}

func TestBioUsesTranslatorDetectionForAmbiguousText(t *testing.T) {
	translator := &fakeTranslator{detected: "fr"}
	idx := &fakeBioIndex{}
	provider := biblioreads.NewBioProvider(&fakeAuthorClient{desc: "Bonjour"}, translator, idx)

	bio, _, err := provider.Bio(context.Background(), goodreadsAuthor(index.AuthorBio{}), "en")
	if err != nil || bio != "[en] Bonjour" {
		t.Fatalf("unexpected bio %q, %v", bio, err)
	}
	if len(translator.calls) != 1 || translator.calls[0].from != translation.AutoDetect {
		t.Fatalf("expected the translator to detect the language, got %+v", translator.calls)
	}
	want := index.AuthorBio{Source: "3389.Stephen_King", Language: "fr", Text: map[string]string{"fr": "Bonjour", "en": "[en] Bonjour"}}
	if len(idx.stored) != 1 || !equalBio(idx.stored[0], want) {
		t.Fatalf("expected the bio to be stored under the detected language, got %+v", idx.stored)
	}
}

func TestBioTranslationFailure(t *testing.T) {
	translator := &fakeTranslator{err: errors.New("unavailable")}
	idx := &fakeBioIndex{}
	provider := biblioreads.NewBioProvider(&fakeAuthorClient{desc: englishBio}, translator, idx)

	bio, translatedFrom, err := provider.Bio(context.Background(), goodreadsAuthor(index.AuthorBio{}), "es")
	if err == nil || !strings.HasPrefix(bio, "Stephen Edwin King") || translatedFrom != "" {
		t.Fatalf("expected the untranslated bio along with the error, got %q, %q, %v", bio, translatedFrom, err)
	}
	if len(idx.stored) != 1 || len(idx.stored[0].Text) != 1 || idx.stored[0].Text["es"] != "" {
		t.Fatalf("only the original bio must be stored, got %+v", idx.stored)
	}

	idx.stored = nil
	provider = biblioreads.NewBioProvider(&fakeAuthorClient{desc: "Bonjour"}, translator, idx)
	if bio, translatedFrom, err := provider.Bio(context.Background(), goodreadsAuthor(index.AuthorBio{}), "es"); err == nil || bio != "Bonjour" || translatedFrom != "" {
		t.Fatalf("expected the original bio of unknown language along with the error, got %q, %q, %v", bio, translatedFrom, err)
	}
	if len(idx.stored) != 0 {
		t.Errorf("a bio in an unknown language must not be stored, got %+v", idx.stored)
	}
}

func TestBioWithoutGoodreadsBio(t *testing.T) {
	client := &fakeAuthorClient{desc: "  <br/> "}
	idx := &fakeBioIndex{}
	provider := biblioreads.NewBioProvider(client, &fakeTranslator{}, idx)

	bio, _, err := provider.Bio(context.Background(), goodreadsAuthor(index.AuthorBio{}), "es")
	if err != nil || bio != "" {
		t.Fatalf("unexpected bio %q, %v", bio, err)
	}
	if len(idx.stored) != 1 || idx.stored[0].Source == "" || len(idx.stored[0].Text) != 0 {
		t.Fatalf("an empty bio must be stored to avoid new lookups, got %+v", idx.stored)
	}
	if bio, _, pending := provider.StoredBio(goodreadsAuthor(idx.stored[0]), "es"); pending || bio != "" {
		t.Errorf("a stored empty bio must not be pending, got %q, %t", bio, pending)
	}
}

func TestBioErrorsAndMissingLinks(t *testing.T) {
	client := &fakeAuthorClient{err: errors.New("unavailable")}
	idx := &fakeBioIndex{}
	provider := biblioreads.NewBioProvider(client, &fakeTranslator{}, idx)
	if _, _, err := provider.Bio(context.Background(), goodreadsAuthor(index.AuthorBio{}), "en"); err == nil {
		t.Error("expected Goodreads errors to be returned")
	}
	if len(idx.stored) != 0 {
		t.Errorf("nothing must be stored after a failed lookup, got %+v", idx.stored)
	}

	unlinked := index.Author{Slug: "jane-doe", Name: "Jane Doe"}
	if bio, _, pending := provider.StoredBio(unlinked, "en"); pending || bio != "" {
		t.Errorf("authors without Goodreads link have no bio, got %q, %t", bio, pending)
	}
	if bio, _, err := provider.Bio(context.Background(), unlinked, "en"); err != nil || bio != "" || len(client.calls) != 1 {
		t.Errorf("authors without Goodreads link must not be looked up, got %q, %v", bio, err)
	}

	stale := goodreadsAuthor(index.AuthorBio{Source: "1.Other", Language: "en", Text: map[string]string{"en": "Old"}})
	if _, _, pending := provider.StoredBio(stale, "en"); !pending {
		t.Error("a bio from a previous Goodreads link must be retrieved again")
	}
}

func TestBioWithoutTranslator(t *testing.T) {
	idx := &fakeBioIndex{}
	provider := biblioreads.NewBioProvider(&fakeAuthorClient{desc: englishBio}, nil, idx)
	bio, _, err := provider.Bio(context.Background(), goodreadsAuthor(index.AuthorBio{}), "es")
	if err != nil || !strings.HasPrefix(bio, "Stephen Edwin King") {
		t.Fatalf("expected the untranslated bio, got %q, %v", bio, err)
	}
	if len(idx.stored) != 1 {
		t.Fatalf("expected the bio to be stored, got %+v", idx.stored)
	}
	if bio, _, pending := provider.StoredBio(goodreadsAuthor(idx.stored[0]), "es"); pending || !strings.HasPrefix(bio, "Stephen Edwin King") {
		t.Errorf("the original bio must be shown when translations are disabled, got %q, %t", bio, pending)
	}
}

func equalBio(a, b index.AuthorBio) bool {
	return a.Source == b.Source && a.Language == b.Language && maps.Equal(a.Text, b.Text)
}

func TestRefreshBioReplacesStoredBio(t *testing.T) {
	client := &fakeAuthorClient{desc: englishBio}
	idx := &fakeBioIndex{}
	provider := biblioreads.NewBioProvider(client, &fakeTranslator{}, idx)

	stale := index.AuthorBio{Source: "3389.Stephen_King", Language: "en", Text: map[string]string{"en": "Old", "es": "Viejo", "fr": "Vieux"}}
	bio, translatedFrom, err := provider.RefreshBio(context.Background(), goodreadsAuthor(stale), "es")
	if err != nil || !strings.HasPrefix(bio, "[es] Stephen Edwin King") || translatedFrom != "en" {
		t.Fatalf("unexpected refreshed bio %q, %q, %v", bio, translatedFrom, err)
	}
	if len(client.calls) != 1 {
		t.Fatalf("a refresh must request Goodreads even if a bio is stored, got %q", client.calls)
	}
	if len(idx.stored) != 0 || len(idx.replaced) != 1 {
		t.Fatalf("a refresh must replace the stored bio, got stored %+v, replaced %+v", idx.stored, idx.replaced)
	}
	if got := idx.replaced[0]; len(got.Text) != 2 || got.Text["fr"] != "" || !strings.HasPrefix(got.Text["en"], "Stephen Edwin King") {
		t.Errorf("stale translations must be discarded, got %+v", got)
	}

	if bio, _, err := provider.RefreshBio(context.Background(), index.Author{Slug: "unlinked"}, "es"); err != nil || bio != "" || len(client.calls) != 1 {
		t.Errorf("authors without Goodreads link must not be refreshed, got %q, %v", bio, err)
	}
}
