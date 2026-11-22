package wikidata

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/svera/coreander/v5/internal/biography"
	"github.com/svera/coreander/v5/internal/datasource/httpjson"
	"github.com/svera/coreander/v5/internal/index"
	gowiki "github.com/trietmn/go-wiki"
	"github.com/trietmn/go-wiki/page"
)

var languageCode = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)*$`)

// go-wiki's language and response cache are global, so all requests must be serialized.
var wikiMu sync.Mutex

// BioSource retrieves biographies from Wikipedia articles linked by Wikidata.
type BioSource struct {
	language string
}

func NewBioSource() *BioSource { return &BioSource{language: "en"} }

func (*BioSource) ForLanguage(language string) biography.Source {
	return &BioSource{language: language}
}

func (s *BioSource) Language() string     { return s.language }
func (s *BioSource) ReferenceKey() string { return "wikipedia:" + s.language }

func (*BioSource) ProfileServiceName() string  { return "Wikipedia" }
func (*BioSource) ProfileInputName() string    { return "wikipediaURL" }
func (*BioSource) MetadataServiceName() string { return "Wikidata" }

func (s *BioSource) ProfileValue(author index.Author) string {
	return author.DataSourceReferences[s.ReferenceKey()]
}

// NormalizeAuthorID accepts article URLs, retaining their language and encoded title.
func (*BioSource) NormalizeAuthorID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	target, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid Wikipedia article URL %q: %w", value, err)
	}
	lang, wikipediaHost := strings.CutSuffix(target.Host, ".wikipedia.org")
	if !wikipediaHost || !languageCode.MatchString(lang) ||
		(target.Scheme != "https" && target.Scheme != "http") ||
		target.User != nil || target.RawQuery != "" || target.ForceQuery ||
		target.Fragment != "" || strings.Contains(value, "#") ||
		!strings.HasPrefix(target.Path, "/wiki/") ||
		strings.TrimSpace(strings.TrimPrefix(target.Path, "/wiki/")) == "" {
		return "", fmt.Errorf("invalid Wikipedia article URL %q", value)
	}
	return value, nil
}

func (*BioSource) CachedBioValid(string) bool { return true }

func (s *BioSource) RetrieveBio(ctx context.Context, id string, _ bool) (string, string, error) {
	normalized, err := s.NormalizeAuthorID(id)
	if err != nil {
		return "", "", err
	}
	if normalized == "" {
		return "", "", fmt.Errorf("missing Wikipedia article URL")
	}
	target, err := url.Parse(normalized)
	if err != nil {
		return "", "", err
	}
	if target.Host != s.language+".wikipedia.org" {
		return "", "", fmt.Errorf("Wikipedia article must be in language %q", s.language)
	}
	wikiMu.Lock()
	defer wikiMu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	gowiki.SetURL("https://%v.wikipedia.org/w/api.php")
	gowiki.SetLanguage(s.language)
	gowiki.SetUserAgent(httpjson.UserAgent)
	// GetPage searches even when suggestions are disabled; use the exact known title instead.
	article, err := page.MakeWikipediaPage(-1, strings.TrimPrefix(target.Path, "/wiki/"), "", true)
	if err != nil {
		return "", "", err
	}
	if article.PageID <= 0 || len(article.Disambiguation) > 0 {
		return "", "", fmt.Errorf("Wikipedia article %q is missing or ambiguous", id)
	}
	text, err := article.GetSummaryHTML()
	if err != nil {
		return "", "", err
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	return biography.CleanHTML(text), id, nil
}
