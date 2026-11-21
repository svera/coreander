package wikidata

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/svera/coreander/v5/internal/biography"
	"github.com/svera/coreander/v5/internal/datasource/httpjson"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/trietmn/go-wiki/models"
	"github.com/trietmn/go-wiki/utils"
)

func stubWiki(t *testing.T, request func(map[string]string) (models.RequestResult, error)) {
	t.Helper()
	previous, lang, target, agent := utils.WikiRequester, utils.WikiLanguage, utils.WikiURL, utils.UserAgent
	utils.WikiRequester = request
	t.Cleanup(func() {
		utils.WikiRequester, utils.WikiLanguage, utils.WikiURL, utils.UserAgent = previous, lang, target, agent
		utils.Cache.Clear()
	})
}

func authorIndex(t *testing.T) *index.BleveIndexer {
	t.Helper()
	documents, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authors, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	idx := index.NewBleve(documents, authors, nil, "", nil, index.Config{})
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

func TestBioUsesKnownLinkInCurrentLanguage(t *testing.T) {
	idx := authorIndex(t)
	author := index.Author{
		Slug: "example", Name: "Example",
		DataSourceLinks: map[string]string{
			"wikipedia:en": "https://en.wikipedia.org/wiki/Example",
			"wikipedia:es": "https://es.wikipedia.org/wiki/Autor_%C3%B1",
		},
		Bio: index.AuthorBio{Source: "goodreads-id", Language: "en", Text: map[string]string{"en": "Old Goodreads bio"}},
	}
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	calls := 0
	summary := `<p><b>Hola</b> &amp; <a href="https://example.com">writer</a> wrote <i>Books</i> and <strong>more</strong> <em>stories</em>.</p><p>Second<br>line<script>alert("unsafe")</script><img src="x" onerror="alert(1)"></p>`
	stubWiki(t, func(args map[string]string) (models.RequestResult, error) {
		calls++
		if utils.UserAgent != httpjson.UserAgent {
			t.Fatalf("Wikipedia requests must use the shared datasource user agent, got %q", utils.UserAgent)
		}
		if utils.WikiLanguage != "es" || utils.WikiURL != "https://%v.wikipedia.org/w/api.php" ||
			args["titles"] != "Autor_\u00f1" || args["list"] != "" {
			t.Fatalf("must request the exact Spanish article without searching: %+v, language=%s", args, utils.WikiLanguage)
		}
		if args["prop"] == "extracts" {
			if _, ok := args["exintro"]; !ok {
				t.Fatal("must request the article summary")
			}
			if _, plaintext := args["explaintext"]; plaintext {
				t.Fatal("must retrieve the HTML summary, not plain text")
			}
		} else if args["prop"] != "info|pageprops" {
			t.Fatalf("unexpected request: %+v", args)
		}
		return models.RequestResult{Query: models.RequestQuery{
			Page: map[string]models.InnerPage{"42": {PageID: 42, Title: "Autor_\u00f1", Extract: summary}},
		}}, nil
	})
	provider := biography.NewService(NewBioSource(), nil, idx)
	if value := provider.ProfileValueForLanguage(author, "es"); value != author.DataSourceLinks["wikipedia:es"] {
		t.Fatalf("profile input must show the current language's link: %q", value)
	}
	if _, _, pending := provider.StoredBio(author, "fr"); pending {
		t.Fatal("must not retrieve a biography without a link in the current language")
	}
	if text, _, err := provider.Bio(context.Background(), author, "fr"); err != nil || text != "" || calls != 0 {
		t.Fatalf("missing link must not trigger requests: %q, %v, calls=%d", text, err, calls)
	}
	want := "<b>Hola</b> &amp; writer wrote <i>Books</i> and <strong>more</strong> <em>stories</em>.\n\nSecond\nline"
	text, from, err := provider.Bio(context.Background(), author, "es")
	if err != nil || text != want || from != "" || calls != 2 {
		t.Fatalf("unexpected biography: %q, %q, %v, calls=%d", text, from, err, calls)
	}
	stored, err := idx.Author(author.Slug, "es")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Bio.Source != author.DataSourceLinks["wikipedia:es"] ||
		stored.Bio.Language != "es" || stored.Bio.Text["es"] != want {
		t.Fatalf("must persist safe emphasis and paragraph breaks with the known language: %+v", stored.Bio)
	}
	if cached, from, pending := provider.StoredBio(stored, "es"); cached != want || from != "" || pending {
		t.Fatalf("unexpected cached biography: %q, %q, %t", cached, from, pending)
	}
	if _, _, err := provider.Bio(context.Background(), stored, "es"); err != nil || calls != 2 {
		t.Fatalf("cached biography must avoid requests: %v, calls=%d", err, calls)
	}
	summary = "Updated."
	if text, _, err := provider.RefreshBio(context.Background(), stored, "es"); err != nil || text != summary || calls != 4 {
		t.Fatalf("refresh must retrieve a fresh summary: %q, %v, calls=%d", text, err, calls)
	}
	stored, err = idx.Author(author.Slug, "es")
	if err != nil || stored.Bio.Text["es"] != summary {
		t.Fatalf("refresh must persist the new summary: %+v, %v", stored.Bio, err)
	}
}

func TestNormalizeBioAuthorID(t *testing.T) {
	source := NewBioSource()
	for _, value := range []string{
		"", "https://es.wikipedia.org/wiki/Autor_%C3%B1",
		"https://zh-min-nan.wikipedia.org/wiki/Example",
	} {
		if got, err := source.NormalizeAuthorID(value); err != nil || got != value {
			t.Errorf("valid URL %q: got %q, %v", value, got, err)
		}
	}
	for _, value := range []string{
		"Example", "https://wikipedia.org/wiki/Example",
		"https://en.wikipedia.org.evil.test/wiki/Example",
		"https://user@en.wikipedia.org/wiki/Example",
		"https://en.wikipedia.org:443/wiki/Example",
		"https://en.wikipedia.org/wiki/", "https://en.wikipedia.org/w/index.php?title=Example",
		"https://en.wikipedia.org/wiki/Example#Life", "https://en.wikipedia.org/wiki/Example?",
	} {
		if _, err := source.NormalizeAuthorID(value); err == nil {
			t.Errorf("must reject %q", value)
		}
	}
}

func TestRetrieveBioFailures(t *testing.T) {
	failure := errors.New("request failed")
	for _, tc := range []struct {
		name     string
		response models.RequestResult
		err      error
	}{
		{name: "network", err: failure},
		{name: "API", response: models.RequestResult{Error: models.RequestError{Code: "failed", Info: "API error"}}},
		{name: "missing", response: models.RequestResult{Query: models.RequestQuery{Page: map[string]models.InnerPage{"-1": {}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubWiki(t, func(map[string]string) (models.RequestResult, error) { return tc.response, tc.err })
			text, canonical, err := NewBioSource().RetrieveBio(context.Background(), "https://en.wikipedia.org/wiki/Example", false)
			if err == nil || text != "" || canonical != "" {
				t.Fatalf("failed lookup must return an error without a biography: %q, %q, %v", text, canonical, err)
			}
		})
	}
}

func TestRetrieveBioRejectsWrongLanguageAndCancellation(t *testing.T) {
	stubWiki(t, func(map[string]string) (models.RequestResult, error) {
		t.Fatal("must not request Wikipedia")
		return models.RequestResult{}, fmt.Errorf("unexpected request")
	})
	source := NewBioSource().ForLanguage("es")
	if _, _, err := source.RetrieveBio(context.Background(), "https://en.wikipedia.org/wiki/Example", false); err == nil {
		t.Fatal("must reject an article in another language")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := source.RetrieveBio(ctx, "https://es.wikipedia.org/wiki/Example", false); !errors.Is(err, context.Canceled) {
		t.Fatalf("must honor cancellation: %v", err)
	}
}

func TestConcurrentBiographiesKeepTheirLanguage(t *testing.T) {
	stubWiki(t, func(args map[string]string) (models.RequestResult, error) {
		if args["titles"] != utils.WikiLanguage {
			return models.RequestResult{}, fmt.Errorf("article %q requested in language %q", args["titles"], utils.WikiLanguage)
		}
		return models.RequestResult{Query: models.RequestQuery{
			Page: map[string]models.InnerPage{"42": {PageID: 42, Extract: utils.WikiLanguage}},
		}}, nil
	})
	results := make(chan error, 2)
	for _, language := range []string{"en", "es"} {
		go func() {
			source := NewBioSource().ForLanguage(language)
			text, _, err := source.RetrieveBio(context.Background(), "https://"+language+".wikipedia.org/wiki/"+language, false)
			if err == nil && text != language {
				err = fmt.Errorf("got biography %q, want language %q", text, language)
			}
			results <- err
		}()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
}
