package biblioreads

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"maps"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/abadojack/whatlanggo"
	"github.com/microcosm-cc/bluemonday"
	"github.com/svera/coreander/v5/internal/datasource/wikidata"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/translation"
	nethtml "golang.org/x/net/html"
	"golang.org/x/sync/singleflight"
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

// Translator translates text from a language, or translation.AutoDetect, to another one,
// returning the translation and the source language.
type Translator interface {
	Translate(ctx context.Context, text, from, to string, gender translation.Gender) (string, string, error)
}

type BioIndex interface {
	StoreAuthorBioForSource(slug, source string, bio index.AuthorBio, merge bool) error
	StoreCanonicalAuthorBio(slug, previousSource string, bio index.AuthorBio) error
}

// BioProvider retrieves author biographies from Goodreads and translates them to the reader's language.
// Biographies and translations are stored in the index, so each one is only requested once.
type BioProvider struct {
	client     AuthorClient
	translator Translator
	index      BioIndex
	lookups    singleflight.Group
}

// NewBioProvider returns a provider that shows biographies untranslated if translator is nil.
func NewBioProvider(client AuthorClient, translator Translator, idx BioIndex) *BioProvider {
	return &BioProvider{client: client, translator: translator, index: idx}
}

// StoredBio returns the indexed biography of author in language without making network requests.
// translatedFrom is the original language of the biography if it is an automatic translation, empty otherwise.
// pending reports that Bio has to be called to retrieve or translate it.
func (p *BioProvider) StoredBio(author index.Author, language string) (bio, translatedFrom string, pending bool) {
	link, ok := authorBioLink(author)
	if !ok {
		return "", "", false
	}
	if author.Bio.Source != link {
		return "", "", true
	}
	if variantNote.MatchString(author.Bio.Text[author.Bio.Language]) {
		return "", "", true
	}
	if text, ok := author.Bio.Text[language]; ok {
		if language != author.Bio.Language {
			translatedFrom = author.Bio.Language
		}
		return text, translatedFrom, false
	}
	original := author.Bio.Text[author.Bio.Language]
	if original == "" || p.translator == nil {
		return original, "", false
	}
	return "", "", true
}

type bioResult struct{ text, translatedFrom string }

// Bio returns the biography of author in language, retrieving and translating it if needed.
// translatedFrom is the original language of the biography if it is an automatic translation, empty otherwise.
// If the translation fails, the biography is returned in its original language along with the error.
func (p *BioProvider) Bio(ctx context.Context, author index.Author, language string) (bio, translatedFrom string, err error) {
	if bio, translatedFrom, pending := p.StoredBio(author, language); !pending {
		return bio, translatedFrom, nil
	}
	// The lookup outlives the request so that its result gets stored even if the reader leaves.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
	defer cancel()
	result, err, _ := p.lookups.Do(author.Slug+"\x00"+language, func() (any, error) {
		text, translatedFrom, err := p.lookup(ctx, author, language, p.bioStore(author, true), false)
		return bioResult{text, translatedFrom}, err
	})
	r, _ := result.(bioResult)
	return r.text, r.translatedFrom, err
}

// RefreshBio retrieves the biography of author from Goodreads again and translates it to language,
// replacing the stored biography and all its translations.
func (p *BioProvider) RefreshBio(ctx context.Context, author index.Author, language string) (bio, translatedFrom string, err error) {
	if _, ok := authorBioLink(author); !ok {
		return "", "", nil
	}
	author.Bio = index.AuthorBio{}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
	defer cancel()
	result, err, _ := p.lookups.Do(author.Slug+"\x00refresh", func() (any, error) {
		text, translatedFrom, err := p.lookup(ctx, author, language, p.bioStore(author, false), true)
		return bioResult{text, translatedFrom}, err
	})
	r, _ := result.(bioResult)
	return r.text, r.translatedFrom, err
}

type storeFunc func(slug string, bio index.AuthorBio) error

func (p *BioProvider) bioStore(author index.Author, merge bool) storeFunc {
	previousSource := author.DataSourceLinks[Source]
	return func(slug string, bio index.AuthorBio) error {
		if bio.Source != previousSource {
			return p.index.StoreCanonicalAuthorBio(slug, previousSource, bio)
		}
		return p.index.StoreAuthorBioForSource(slug, previousSource, bio, merge)
	}
}

// UpdateBio retrieves a biography from a new Goodreads profile, saving the link with the result.
func (p *BioProvider) UpdateBio(ctx context.Context, author index.Author, id, language string) (string, string, error) {
	if id == "" {
		if err := p.index.StoreCanonicalAuthorBio(author.Slug, author.DataSourceLinks[Source], index.AuthorBio{}); err != nil {
			return "", "", fmt.Errorf("clearing Goodreads author link: %w", err)
		}
		return "", "", nil
	}
	if !authorIDPattern.MatchString(id) {
		return "", "", fmt.Errorf("invalid Goodreads author ID %q", id)
	}
	save := p.bioStore(author, false)
	author.DataSourceLinks = maps.Clone(author.DataSourceLinks)
	if author.DataSourceLinks == nil {
		author.DataSourceLinks = make(map[string]string)
	}
	author.DataSourceLinks[Source] = id
	author.Bio = index.AuthorBio{}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
	defer cancel()
	result, err, _ := p.lookups.Do(author.Slug+"\x00update\x00"+id+"\x00"+language, func() (any, error) {
		text, from, err := p.lookup(ctx, author, language, save, true)
		return bioResult{text, from}, err
	})
	bio := result.(bioResult)
	return bio.text, bio.translatedFrom, err
}

func (p *BioProvider) lookup(ctx context.Context, author index.Author, language string, save storeFunc, refresh bool) (string, string, error) {
	link, _ := authorBioLink(author)
	bio := author.Bio
	original := bio.Text[bio.Language]
	if bio.Source != link || variantNote.MatchString(original) {
		response, canonical, err := p.resolveAuthor(ctx, link, refresh)
		if err != nil {
			return "", "", fmt.Errorf("retrieving Goodreads author %s: %w", link, err)
		}
		link = canonical
		bio = index.AuthorBio{Source: link}
		if original = bioText(response.Desc); original != "" {
			bio.Language = detectLanguage(html.UnescapeString(bluemonday.StrictPolicy().Sanitize(original)))
		}
		if bio.Language != "" {
			bio.Text = map[string]string{bio.Language: original}
		}
	}
	// A bio of unknown language is not stored, as a source without texts means it has no bio.
	if original == "" || bio.Language == language || (p.translator == nil && bio.Language != "") {
		return original, "", store(save, author.Slug, bio)
	}
	if p.translator == nil {
		return original, "", nil
	}

	from := bio.Language
	if from == "" {
		from = translation.AutoDetect
	}
	var gender translation.Gender
	switch author.Gender {
	case wikidata.GenderMale, wikidata.GenderTrasgenderMale:
		gender = translation.GenderMasculine
	case wikidata.GenderFemale, wikidata.GenderTrasgenderFemale:
		gender = translation.GenderFeminine
	}
	translated, detected, err := p.translateBio(ctx, original, from, language, gender)
	if err != nil {
		if bio.Language != "" {
			err = errors.Join(err, store(save, author.Slug, bio))
		}
		return original, "", err
	}
	if bio.Language == "" {
		bio.Language = detected
		bio.Text = map[string]string{detected: original}
	}
	if bio.Language == language {
		return original, "", store(save, author.Slug, bio)
	}
	bio.Text[language] = translated
	return translated, bio.Language, store(save, author.Slug, bio)
}

func (p *BioProvider) resolveAuthor(ctx context.Context, link string, refresh bool) (AuthorResponse, string, error) {
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
			response, err = p.client.RefreshAuthor(ctx, link)
		} else {
			response, err = p.client.Author(ctx, link)
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

func (p *BioProvider) translateBio(ctx context.Context, text, from, to string, gender translation.Gender) (string, string, error) {
	tokens := nethtml.NewTokenizer(strings.NewReader(text))
	var result strings.Builder
	for {
		switch tokens.Next() {
		case nethtml.ErrorToken:
			if tokens.Err() != io.EOF {
				return "", "", tokens.Err()
			}
			return result.String(), from, nil
		case nethtml.TextToken:
			part := string(tokens.Text())
			if strings.ContainsFunc(part, unicode.IsLetter) {
				leading := part[:len(part)-len(strings.TrimLeftFunc(part, unicode.IsSpace))]
				trailing := part[len(strings.TrimRightFunc(part, unicode.IsSpace)):]
				translated, detected, err := p.translator.Translate(ctx, strings.TrimSpace(part), from, to, gender)
				if err != nil {
					return "", "", err
				}
				from = detected
				part = leading + translated + trailing
			}
			result.WriteString(html.EscapeString(part))
		case nethtml.StartTagToken, nethtml.EndTagToken:
			result.Write(tokens.Raw())
		}
	}
}

func store(save storeFunc, slug string, bio index.AuthorBio) error {
	if err := save(slug, bio); err != nil {
		return fmt.Errorf("storing biography for author %s: %w", slug, err)
	}
	return nil
}

func authorBioLink(author index.Author) (string, bool) {
	link := author.DataSourceLinks[Source]
	return link, authorIDPattern.MatchString(link)
}

// detectLanguage returns the ISO 639-1 code of the language text is written in, or an empty
// string if it cannot be reliably detected.
func detectLanguage(text string) string {
	info := whatlanggo.Detect(text)
	if !info.IsReliable() {
		return ""
	}
	return info.Lang.Iso6391()
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
