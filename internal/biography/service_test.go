package biography_test

import (
	"context"
	"errors"
	"html"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/svera/coreander/v5/internal/biography"
	"github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/index"
	gt "gopkg.gilang.dev/translator/v2"
)

const (
	englishBio = "Stephen Edwin King was born the second son of Donald and Nellie Ruth Pillsbury King.\n\n" +
		"After his father left them when Stephen was two, he and his older brother, David, were raised by his mother. Parts of his childhood were spent in Fort Wayne, Indiana &amp; Stratford."
	spanishBio = "Arturo Pérez-Reverte es un escritor y periodista español, miembro de la Real Academia Española. Fue corresponsal de guerra durante más de veinte años y es autor de numerosas novelas."
)

type fakeSource struct {
	desc, canonical, invalidCached string
	err                            error
	calls                          []string
	refreshes                      []bool
}

func (*fakeSource) ReferenceKey() string       { return "catalog" }
func (*fakeSource) ProfileServiceName() string { return "Example catalog" }
func (*fakeSource) ProfileInputName() string   { return "catalogID" }
func (*fakeSource) ProfileValue(author index.Author) string {
	return author.DataSourceReferences["catalog"]
}
func (*fakeSource) NormalizeAuthorID(value string) (string, error) {
	if value == "invalid" {
		return "", errors.New("invalid catalog ID")
	}
	return strings.TrimSpace(value), nil
}
func (s *fakeSource) CachedBioValid(text string) bool {
	return s.invalidCached == "" || text != s.invalidCached
}
func (s *fakeSource) RetrieveBio(_ context.Context, id string, refresh bool) (string, string, error) {
	s.calls = append(s.calls, id)
	s.refreshes = append(s.refreshes, refresh)
	canonical := id
	if s.canonical != "" {
		canonical = s.canonical
	}
	return s.desc, canonical, s.err
}

type translateCall struct {
	text, from, to string
	gender         gt.Gender
}

type fakeTranslator struct {
	detected  string
	err       error
	calls     []translateCall
	nilResult bool
	empty     bool
}

func (f *fakeTranslator) Translate(ctx context.Context, text, from, to string) (*gt.Translated, error) {
	return f.TranslateWithGender(ctx, text, from, to, gt.GenderUnknown)
}

func (f *fakeTranslator) TranslateWithGender(_ context.Context, text, from, to string, gender gt.Gender) (*gt.Translated, error) {
	f.calls = append(f.calls, translateCall{text, from, to, gender})
	if f.err != nil {
		return nil, f.err
	}
	if f.nilResult {
		return nil, nil
	}
	if f.empty {
		return &gt.Translated{}, nil
	}
	if from == "auto" || f.detected != "" {
		from = f.detected
	}
	return &gt.Translated{
		Text: "[" + to + "] " + text,
		From: gt.TranslateFrom{Language: gt.TranslateFromLanguage{Iso: from}},
	}, nil
}

type fakeBioIndex struct {
	stored, replaced        []index.AuthorBio
	linkKey, previousSource string
	err                     error
}

func (f *fakeBioIndex) StoreCanonicalAuthorBio(_ string, linkKey, previousSource string, bio index.AuthorBio) error {
	f.linkKey, f.previousSource = linkKey, previousSource
	f.replaced = append(f.replaced, bio)
	return f.err
}
func (f *fakeBioIndex) StoreAuthorBioForLink(_ string, linkKey, previousSource string, bio index.AuthorBio, merge bool) error {
	f.linkKey, f.previousSource = linkKey, previousSource
	if merge {
		f.stored = append(f.stored, bio)
	} else {
		f.replaced = append(f.replaced, bio)
	}
	return f.err
}

func catalogAuthor(bio index.AuthorBio) index.Author {
	return index.Author{
		Slug: "alice", Name: "Alice Example",
		DataSourceReferences: map[string]string{"catalog": "alice"}, Bio: bio,
	}
}

func equalBio(a, b index.AuthorBio) bool {
	return a.Source == b.Source && a.Language == b.Language && maps.Equal(a.Text, b.Text)
}

func TestBioRetrievesAndTranslatesBio(t *testing.T) {
	for _, tt := range []struct {
		name, original, spanish, french string
		calls                           []translateCall
	}{
		{
			name: "plain text", original: englishBio,
			spanish: "[es] " + englishBio, french: "[fr] " + englishBio,
			calls: []translateCall{{html.UnescapeString(englishBio), "en", "es", gt.GenderUnknown}},
		},
		{
			name:     "emphasis",
			original: `<b>Isaac Asimov</b> wrote <i>Foundation</i> and <strong>many</strong> <em>books</em>.`,
			spanish:  `<b>[es] Isaac Asimov</b> [es] wrote <i>[es] Foundation</i> [es] and <strong>[es] many</strong> <em>[es] books</em>.`,
			french:   `<b>[fr] Isaac Asimov</b> [fr] wrote <i>[fr] Foundation</i> [fr] and <strong>[fr] many</strong> <em>[fr] books</em>.`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := &fakeSource{desc: tt.original}
			translator := &fakeTranslator{detected: "en"}
			idx := &fakeBioIndex{}
			provider := biography.NewService(source, translator, idx)
			author := catalogAuthor(index.AuthorBio{})
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
			if !slices.Equal(source.calls, []string{"alice"}) || !slices.Equal(source.refreshes, []bool{false}) {
				t.Fatalf("expected one ordinary source request, got %q, refresh=%v", source.calls, source.refreshes)
			}
			if tt.calls != nil && !slices.Equal(translator.calls, tt.calls) {
				t.Fatalf("expected plain text to be translated from its detected language, got %+v", translator.calls)
			}
			if bio != tt.spanish {
				t.Fatalf("unexpected bio %q", bio)
			}
			want := index.AuthorBio{Source: "alice", Language: "en", Text: map[string]string{"en": tt.original, "es": tt.spanish}}
			if len(idx.stored) != 1 || !equalBio(idx.stored[0], want) || idx.linkKey != "catalog" {
				t.Fatalf("expected original and translation to be stored for the source link, got %+v", idx)
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
			if bio, _, err := provider.Bio(context.Background(), author, "fr"); err != nil || bio != tt.french {
				t.Fatalf("unexpected French bio %q, %v", bio, err)
			}
			if len(source.calls) != 1 {
				t.Error("translating a stored bio must not request the source again")
			}
			if last := idx.stored[len(idx.stored)-1]; last.Language != "en" || last.Text["fr"] != tt.french {
				t.Errorf("expected the French translation to be stored, got %+v", last)
			}
		})
	}
}

func TestBioTranslationUsesAuthorGender(t *testing.T) {
	for _, tt := range []struct {
		name   string
		gender float64
		want   gt.Gender
	}{
		{"male", model.GenderMale, gt.GenderMasculine},
		{"female", model.GenderFemale, gt.GenderFeminine},
		{"transgender male", model.GenderTransgenderMale, gt.GenderMasculine},
		{"transgender female", model.GenderTransgenderFemale, gt.GenderFeminine},
		{"intersex", model.GenderIntersex, gt.GenderUnknown},
		{"unknown", model.GenderUnknown, gt.GenderUnknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			translator := &fakeTranslator{detected: "fr"}
			provider := biography.NewService(&fakeSource{}, translator, &fakeBioIndex{})
			author := catalogAuthor(index.AuthorBio{
				Source: "alice", Language: "en",
				Text: map[string]string{"en": "A writer wrote <i>many books</i> for readers."},
			})
			author.Gender = tt.gender
			if _, _, err := provider.Bio(context.Background(), author, "es"); err != nil {
				t.Fatal(err)
			}
			if len(translator.calls) != 3 {
				t.Fatalf("expected three translated text fragments, got %d", len(translator.calls))
			}
			for _, call := range translator.calls {
				if call.gender != tt.want {
					t.Errorf("gender for %q: got %v, want %v", call.text, call.gender, tt.want)
				}
				if call.from != "en" {
					t.Errorf("the known source language must be preserved, got %q", call.from)
				}
			}
		})
	}
}

func TestBioInReaderLanguageIsNotTranslated(t *testing.T) {
	translator := &fakeTranslator{}
	idx := &fakeBioIndex{}
	provider := biography.NewService(&fakeSource{desc: spanishBio}, translator, idx)
	bio, translatedFrom, err := provider.Bio(context.Background(), catalogAuthor(index.AuthorBio{}), "es")
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
	for _, tt := range []struct {
		detected, language string
	}{
		{"fr", "fr"},
		{" FR-fr ", "fr"},
		{"zh-CN", "zh"},
		{" PT ", "pt"},
	} {
		t.Run(tt.detected, func(t *testing.T) {
			translator := &fakeTranslator{detected: tt.detected}
			idx := &fakeBioIndex{}
			provider := biography.NewService(&fakeSource{desc: "Bonjour"}, translator, idx)
			bio, from, err := provider.Bio(context.Background(), catalogAuthor(index.AuthorBio{}), "en")
			if err != nil || bio != "[en] Bonjour" || from != tt.language {
				t.Fatalf("unexpected bio %q from %q, %v", bio, from, err)
			}
			if len(translator.calls) != 1 || translator.calls[0].from != "auto" {
				t.Fatalf("expected the translator to detect the language, got %+v", translator.calls)
			}
			want := index.AuthorBio{Source: "alice", Language: tt.language, Text: map[string]string{tt.language: "Bonjour", "en": "[en] Bonjour"}}
			if len(idx.stored) != 1 || !equalBio(idx.stored[0], want) {
				t.Fatalf("expected the bio to be stored under the normalized detected language, got %+v", idx.stored)
			}
		})
	}
}

func TestBioRejectsInvalidTranslationResults(t *testing.T) {
	for _, tt := range []struct {
		name       string
		translator fakeTranslator
		message    string
	}{
		{"nil result", fakeTranslator{nilResult: true}, "empty translation"},
		{"empty result", fakeTranslator{empty: true}, "empty translation"},
		{"missing language", fakeTranslator{}, "source language not detected"},
		{"blank language", fakeTranslator{detected: " \t "}, "source language not detected"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			idx := &fakeBioIndex{}
			provider := biography.NewService(&fakeSource{desc: "Bonjour"}, &tt.translator, idx)
			bio, from, err := provider.Bio(context.Background(), catalogAuthor(index.AuthorBio{}), "en")
			if err == nil || !strings.Contains(err.Error(), tt.message) || bio != "Bonjour" || from != "" {
				t.Fatalf("expected original text and an explicit translation error: %q, %q, %v", bio, from, err)
			}
			if len(idx.stored) != 0 || len(idx.replaced) != 0 {
				t.Fatal("invalid translations must not be stored")
			}
		})
	}
}

func TestBioUnsupportedGenderPreference(t *testing.T) {
	idx := &fakeBioIndex{}
	provider := biography.NewService(&fakeSource{desc: englishBio}, gt.NewDeepLTranslator(), idx)
	author := catalogAuthor(index.AuthorBio{})
	author.Gender = model.GenderMale
	bio, from, err := provider.Bio(context.Background(), author, "es")
	if !errors.Is(err, gt.ErrUnsupportedOption) || bio != englishBio || from != "" {
		t.Fatalf("expected the original bio and unsupported-option error: %q, %q, %v", bio, from, err)
	}
	if len(idx.stored) != 1 || len(idx.stored[0].Text) != 1 || idx.stored[0].Text["en"] != englishBio {
		t.Fatalf("only the original bio must be cached after an unsupported preference: %+v", idx.stored)
	}
}

func TestBioTranslationFailure(t *testing.T) {
	translator := &fakeTranslator{err: errors.New("unavailable")}
	idx := &fakeBioIndex{}
	provider := biography.NewService(&fakeSource{desc: englishBio}, translator, idx)
	bio, translatedFrom, err := provider.Bio(context.Background(), catalogAuthor(index.AuthorBio{}), "es")
	if err == nil || bio != englishBio || translatedFrom != "" {
		t.Fatalf("expected the untranslated bio along with the error, got %q, %q, %v", bio, translatedFrom, err)
	}
	if len(idx.stored) != 1 || len(idx.stored[0].Text) != 1 || idx.stored[0].Text["es"] != "" {
		t.Fatalf("only the original bio must be stored, got %+v", idx.stored)
	}
	idx.stored = nil
	provider = biography.NewService(&fakeSource{desc: "Bonjour"}, translator, idx)
	if bio, translatedFrom, err := provider.Bio(context.Background(), catalogAuthor(index.AuthorBio{}), "es"); err == nil || bio != "Bonjour" || translatedFrom != "" {
		t.Fatalf("expected the original bio of unknown language along with the error, got %q, %q, %v", bio, translatedFrom, err)
	}
	if len(idx.stored) != 0 {
		t.Errorf("a bio in an unknown language must not be stored, got %+v", idx.stored)
	}
}

func TestBioWithoutBiography(t *testing.T) {
	source := &fakeSource{}
	idx := &fakeBioIndex{}
	provider := biography.NewService(source, &fakeTranslator{}, idx)
	bio, _, err := provider.Bio(context.Background(), catalogAuthor(index.AuthorBio{}), "es")
	if err != nil || bio != "" {
		t.Fatalf("unexpected bio %q, %v", bio, err)
	}
	if len(idx.stored) != 1 || idx.stored[0].Source == "" || len(idx.stored[0].Text) != 0 {
		t.Fatalf("an empty bio must be stored to avoid new lookups, got %+v", idx.stored)
	}
	if bio, _, pending := provider.StoredBio(catalogAuthor(idx.stored[0]), "es"); pending || bio != "" {
		t.Errorf("a stored empty bio must not be pending, got %q, %t", bio, pending)
	}
}

func TestBioErrorsAndMissingLinks(t *testing.T) {
	source := &fakeSource{err: errors.New("unavailable")}
	idx := &fakeBioIndex{}
	provider := biography.NewService(source, &fakeTranslator{}, idx)
	if _, _, err := provider.Bio(context.Background(), catalogAuthor(index.AuthorBio{}), "en"); !errors.Is(err, source.err) {
		t.Errorf("expected source errors to be returned, got %v", err)
	}
	if len(idx.stored) != 0 {
		t.Errorf("nothing must be stored after a failed lookup, got %+v", idx.stored)
	}
	unlinked := index.Author{Slug: "jane-doe", Name: "Jane Doe"}
	if bio, _, pending := provider.StoredBio(unlinked, "en"); pending || bio != "" {
		t.Errorf("authors without source link have no bio, got %q, %t", bio, pending)
	}
	if bio, _, err := provider.Bio(context.Background(), unlinked, "en"); err != nil || bio != "" || len(source.calls) != 1 {
		t.Errorf("authors without source link must not be looked up, got %q, %v", bio, err)
	}
	stale := catalogAuthor(index.AuthorBio{Source: "other", Language: "en", Text: map[string]string{"en": "Old"}})
	if _, _, pending := provider.StoredBio(stale, "en"); !pending {
		t.Error("a bio from a previous source link must be retrieved again")
	}
	source = &fakeSource{desc: englishBio, canonical: "invalid"}
	provider = biography.NewService(source, nil, idx)
	if _, _, err := provider.Bio(context.Background(), catalogAuthor(index.AuthorBio{}), "en"); err == nil ||
		len(idx.stored) != 0 || len(idx.replaced) != 0 {
		t.Fatal("invalid canonical profiles must fail without replacing the stored link or bio")
	}
}

func TestBioWithoutTranslator(t *testing.T) {
	idx := &fakeBioIndex{}
	provider := biography.NewService(&fakeSource{desc: englishBio}, nil, idx)
	bio, _, err := provider.Bio(context.Background(), catalogAuthor(index.AuthorBio{}), "es")
	if err != nil || bio != englishBio {
		t.Fatalf("expected the untranslated bio, got %q, %v", bio, err)
	}
	if len(idx.stored) != 1 {
		t.Fatalf("expected the bio to be stored, got %+v", idx.stored)
	}
	if bio, _, pending := provider.StoredBio(catalogAuthor(idx.stored[0]), "es"); pending || bio != englishBio {
		t.Errorf("the original bio must be shown when translations are disabled, got %q, %t", bio, pending)
	}
}

func TestRefreshBioReplacesStoredBio(t *testing.T) {
	source := &fakeSource{desc: englishBio}
	idx := &fakeBioIndex{}
	provider := biography.NewService(source, &fakeTranslator{}, idx)
	stale := index.AuthorBio{Source: "alice", Language: "en", Text: map[string]string{"en": "Old", "es": "Viejo", "fr": "Vieux"}}
	bio, translatedFrom, err := provider.RefreshBio(context.Background(), catalogAuthor(stale), "es")
	if err != nil || bio != "[es] "+englishBio || translatedFrom != "en" {
		t.Fatalf("unexpected refreshed bio %q, %q, %v", bio, translatedFrom, err)
	}
	if !slices.Equal(source.calls, []string{"alice"}) || !slices.Equal(source.refreshes, []bool{true}) {
		t.Fatalf("a refresh must request a fresh source bio, got %q, refresh=%v", source.calls, source.refreshes)
	}
	if len(idx.stored) != 0 || len(idx.replaced) != 1 {
		t.Fatalf("a refresh must replace the stored bio, got stored %+v, replaced %+v", idx.stored, idx.replaced)
	}
	if got := idx.replaced[0]; len(got.Text) != 2 || got.Text["fr"] != "" || got.Text["en"] != englishBio {
		t.Errorf("stale translations must be discarded, got %+v", got)
	}
	if bio, _, err := provider.RefreshBio(context.Background(), index.Author{Slug: "unlinked"}, "es"); err != nil || bio != "" || len(source.calls) != 1 {
		t.Errorf("authors without source link must not be refreshed, got %q, %v", bio, err)
	}
}

func TestUpdateBio(t *testing.T) {
	for _, previous := range []string{"", "alice"} {
		t.Run(previous, func(t *testing.T) {
			source := &fakeSource{desc: englishBio, canonical: "primary"}
			idx := &fakeBioIndex{}
			provider := biography.NewService(source, &fakeTranslator{}, idx)
			author := index.Author{Slug: "alice", DataSourceReferences: map[string]string{"catalog": previous}}
			text, _, err := provider.UpdateBio(context.Background(), author, "variant", "es")
			if err != nil || text != "[es] "+englishBio || idx.linkKey != "catalog" || idx.previousSource != previous ||
				len(idx.replaced) != 1 || idx.replaced[0].Source != "primary" {
				t.Fatalf("editing a source ID must save its canonical profile and bio: %q, %v, %+v", text, err, idx)
			}
			if !slices.Equal(source.calls, []string{"variant"}) || !slices.Equal(source.refreshes, []bool{true}) {
				t.Fatalf("profile edits must bypass source caches, got %q, refresh=%v", source.calls, source.refreshes)
			}
			if author.DataSourceReferences["catalog"] != previous {
				t.Fatal("updating must not mutate the caller's author")
			}
		})
	}
	source := &fakeSource{err: errors.New("unavailable")}
	idx := &fakeBioIndex{}
	provider := biography.NewService(source, nil, idx)
	if _, _, err := provider.UpdateBio(context.Background(), catalogAuthor(index.AuthorBio{}), "other", "es"); err == nil || len(idx.replaced) != 0 || len(idx.stored) != 0 {
		t.Fatal("failed fetches must preserve the existing bio and link")
	}
	if _, _, err := provider.UpdateBio(context.Background(), catalogAuthor(index.AuthorBio{}), "invalid", "es"); err == nil || len(source.calls) != 1 {
		t.Fatal("invalid source IDs must fail before retrieval")
	}
	if _, _, err := provider.UpdateBio(context.Background(), catalogAuthor(index.AuthorBio{}), "", "es"); err != nil ||
		len(idx.replaced) != 1 || idx.replaced[0].Source != "" || idx.linkKey != "catalog" || idx.previousSource != "alice" {
		t.Fatalf("removing a profile must clear its source link and bio: %v, %+v", err, idx)
	}
}

func TestBioInvalidCachedTextResolvesCanonicalProfile(t *testing.T) {
	source := &fakeSource{desc: englishBio, canonical: "primary", invalidCached: "obsolete"}
	idx := &fakeBioIndex{}
	provider := biography.NewService(source, &fakeTranslator{}, idx)
	author := catalogAuthor(index.AuthorBio{
		Source: "alice", Language: "en", Text: map[string]string{"en": "obsolete", "es": "old translation"},
	})
	if _, _, pending := provider.StoredBio(author, "es"); !pending {
		t.Fatal("source-invalid cached biographies must be retrieved again")
	}
	bio, _, err := provider.Bio(context.Background(), author, "es")
	if err != nil || bio != "[es] "+englishBio || len(idx.stored) != 0 || len(idx.replaced) != 1 ||
		idx.replaced[0].Source != "primary" || idx.previousSource != "alice" || idx.linkKey != "catalog" {
		t.Fatalf("canonical profile and bio must replace the invalid cache: %q, %v, %+v", bio, err, idx)
	}
}

func TestBioStorageErrors(t *testing.T) {
	sentinel := errors.New("storage failed")
	idx := &fakeBioIndex{err: sentinel}
	provider := biography.NewService(&fakeSource{desc: englishBio}, nil, idx)
	author := catalogAuthor(index.AuthorBio{})
	if _, _, err := provider.Bio(context.Background(), author, "en"); !errors.Is(err, sentinel) {
		t.Fatalf("storage errors must be returned: %v", err)
	}
	if _, _, err := provider.UpdateBio(context.Background(), author, "", "en"); !errors.Is(err, sentinel) {
		t.Fatalf("profile removal errors must be returned: %v", err)
	}
}
