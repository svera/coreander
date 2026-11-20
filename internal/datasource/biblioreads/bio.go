package biblioreads

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/svera/coreander/v5/internal/index"
	nethtml "golang.org/x/net/html"
)

const goodreadsAuthorPrefix = "/author/show/"

var variantNote = regexp.MustCompile(`(?i)^librarian note:\s*this is an alternate name variant profile\.\s*see primary profile:`)
var primaryProfilePrefix = regexp.MustCompile(`(?i)^librarian note:\s*this is an alternate name variant profile\.\s*see primary profile:\s*$`)
var authorIDPattern = regexp.MustCompile(`^[1-9][0-9]*(?:[.][A-Za-z0-9_~-]+)?$`)
var authorProfilePath = regexp.MustCompile(`^/author/show/([1-9][0-9]*)(?:[.][A-Za-z0-9_~-]+)?/?$`)

var bioLineBreak = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>`)
var bioFormatting = bluemonday.NewPolicy().AllowElements("b", "strong", "i", "em")

// spaceBeforePunctuation matches whitespace left before punctuation by inline elements, as in "<i>Title </i>,".
var spaceBeforePunctuation = regexp.MustCompile(` +([,.;:!?)\]])`)

type AuthorClient interface {
	Author(ctx context.Context, id string) (AuthorResponse, error)
	RefreshAuthor(ctx context.Context, id string) (AuthorResponse, error)
}

// BioSource retrieves and cleans Goodreads biographies.
type BioSource struct {
	client AuthorClient
}

func NewBioSource(client AuthorClient) *BioSource {
	return &BioSource{client: client}
}

func (*BioSource) LinkKey() string { return Source }

func (*BioSource) ProfileServiceName() string {
	return "Goodreads"
}

func (*BioSource) ProfileInputName() string {
	return "goodreadsID"
}

func (*BioSource) ProfileValue(author index.Author) string {
	if id := author.DataSourceLinks[Source]; id != "" {
		return goodreadsBaseURL + goodreadsAuthorPrefix + id
	}
	return ""
}

// NormalizeAuthorID accepts a Goodreads author ID or profile URL.
func (*BioSource) NormalizeAuthorID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "://") {
		target, err := url.Parse(value)
		if err != nil || (target.Scheme != "https" && target.Scheme != "http") ||
			(target.Host != "www.goodreads.com" && target.Host != "goodreads.com") ||
			target.User != nil || target.RawQuery != "" || target.Fragment != "" ||
			!strings.HasPrefix(target.Path, goodreadsAuthorPrefix) {
			return "", fmt.Errorf("invalid Goodreads author URL %q", value)
		}
		value = strings.TrimSuffix(strings.TrimPrefix(target.Path, goodreadsAuthorPrefix), "/")
	}
	if value != "" && !authorIDPattern.MatchString(value) {
		return "", fmt.Errorf("invalid Goodreads author ID %q", value)
	}
	return value, nil
}

func (*BioSource) CachedBioValid(text string) bool {
	return !variantNote.MatchString(text)
}

// RetrieveBio resolves primary profiles and returns safe HTML and the canonical profile ID.
// refresh bypasses the client's cached responses for every profile in the chain.
func (s *BioSource) RetrieveBio(ctx context.Context, id string, refresh bool) (text, canonicalID string, err error) {
	response, canonical, err := s.resolveAuthor(ctx, id, refresh)
	if err != nil {
		return "", "", err
	}
	return bioText(response.Desc), canonical, nil
}

func (s *BioSource) resolveAuthor(ctx context.Context, link string, refresh bool) (AuthorResponse, string, error) {
	visited := make(map[string]bool)
	for range 5 {
		match := authorProfilePath.FindStringSubmatch(goodreadsAuthorPrefix + link)
		if match == nil {
			return AuthorResponse{}, "", fmt.Errorf("invalid Goodreads author link %q", link)
		}
		if visited[match[1]] {
			return AuthorResponse{}, "", fmt.Errorf("Goodreads primary profile redirect loop at %s", link)
		}
		visited[match[1]] = true
		var response AuthorResponse
		var err error
		if refresh {
			response, err = s.client.RefreshAuthor(ctx, link)
		} else {
			response, err = s.client.Author(ctx, link)
		}
		if err != nil {
			return AuthorResponse{}, "", err
		}
		next, err := primaryProfile(response.Desc)
		if err != nil {
			return AuthorResponse{}, "", err
		}
		if next == "" {
			return response, link, nil
		}
		link = next
	}
	return AuthorResponse{}, "", errors.New("Goodreads primary profile redirect limit exceeded")
}

func primaryProfile(desc string) (string, error) {
	tokens := nethtml.NewTokenizer(strings.NewReader(desc))
	var text strings.Builder
	for {
		switch tokens.Next() {
		case nethtml.ErrorToken:
			if variantNote.MatchString(strings.Join(strings.Fields(text.String()), " ")) {
				return "", errors.New("Goodreads variant note has no valid primary profile link")
			}
			return "", nil
		case nethtml.TextToken:
			text.Write(tokens.Text())
		case nethtml.StartTagToken:
			token := tokens.Token()
			if token.Data != "a" || !primaryProfilePrefix.MatchString(strings.Join(strings.Fields(text.String()), " ")) {
				continue
			}
			for _, attr := range token.Attr {
				if attr.Key != "href" {
					continue
				}
				target, err := url.Parse(attr.Val)
				if err != nil || target.User != nil || target.RawQuery != "" || target.Fragment != "" ||
					(target.Host != "" && target.Host != "www.goodreads.com" && target.Host != "goodreads.com") ||
					(target.Scheme != "" && target.Scheme != "https" && target.Scheme != "http") ||
					!authorProfilePath.MatchString(target.Path) {
					return "", fmt.Errorf("invalid Goodreads primary profile link %q", attr.Val)
				}
				return strings.TrimSuffix(strings.TrimPrefix(target.Path, goodreadsAuthorPrefix), "/"), nil
			}
			return "", errors.New("Goodreads primary profile link has no href")
		}
	}
}

// bioText retains safe emphasis, keeping line breaks and collapsing runs
// of blank lines into one, which separates paragraphs. As in HTML, newlines in the source are
// just whitespace, only elements such as <br> or <p> break lines.
func bioText(desc string) string {
	text := bioLineBreak.ReplaceAllString(strings.Join(strings.Fields(desc), " "), "\n")
	text = bioFormatting.Sanitize(text)
	var lines []string
	blank := false
	for line := range strings.SplitSeq(text, "\n") {
		line = spaceBeforePunctuation.ReplaceAllString(strings.Join(strings.Fields(line), " "), "$1")
		if line == "" {
			blank = len(lines) > 0
			continue
		}
		if blank {
			lines = append(lines, "")
			blank = false
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
