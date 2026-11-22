// Package biography retrieves and translates author biographies independently of their source.
package biography

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"maps"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/abadojack/whatlanggo"
	"github.com/microcosm-cc/bluemonday"
	"github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/index"
	nethtml "golang.org/x/net/html"
	"golang.org/x/sync/singleflight"
	gt "gopkg.gilang.dev/translator/v2"
)

const requestTimeout = 3 * time.Minute

var paragraphEnd = regexp.MustCompile(`(?i)</p>`)
var htmlLineBreak = regexp.MustCompile(`(?i)<br\s*/?>|</div>`)
var htmlFormatting = bluemonday.NewPolicy().AllowElements("b", "strong", "i", "em")

// spaceBeforePunctuation matches whitespace left by inline elements, as in "<i>Title </i>,".
var spaceBeforePunctuation = regexp.MustCompile(` +([,.;:!?)\]])`)

// Source supplies cleaned biography HTML (b, strong, i and em, with newline-separated
// paragraphs) and identifies the profile it belongs to.
type Source interface {
	LinkKey() string
	ProfileServiceName() string
	ProfileInputName() string
	ProfileValue(author index.Author) string
	NormalizeAuthorID(value string) (string, error)
	CachedBioValid(text string) bool
	RetrieveBio(ctx context.Context, id string, refresh bool) (text, canonicalID string, err error)
}

// LocalizedSource supplies biographies directly in the requested language.
type LocalizedSource interface {
	ForLanguage(language string) Source
	Language() string
}

type Index interface {
	StoreAuthorBioForLink(slug, linkKey, expectedLink string, bio index.AuthorBio, merge bool) error
	StoreCanonicalAuthorBio(slug, linkKey, previousLink string, bio index.AuthorBio) error
}

// Service manages biography retrieval, translation and indexed originals/translations.
type Service struct {
	source     Source
	translator gt.Translator
	index      Index
	lookups    *singleflight.Group
}

// NewService shows biographies untranslated when translator is nil.
func NewService(source Source, translator gt.Translator, idx Index) *Service {
	return &Service{source: source, translator: translator, index: idx, lookups: &singleflight.Group{}}
}

func (s *Service) forLanguage(language string) *Service {
	if source, ok := s.source.(LocalizedSource); ok {
		return &Service{source: source.ForLanguage(language), translator: s.translator, index: s.index, lookups: s.lookups}
	}
	return s
}

func (s *Service) LinkKey() string            { return s.source.LinkKey() }
func (s *Service) ProfileServiceName() string { return s.source.ProfileServiceName() }
func (s *Service) ProfileInputName() string   { return s.source.ProfileInputName() }
func (s *Service) ProfileValue(author index.Author) string {
	return s.source.ProfileValue(author)
}
func (s *Service) ProfileValueForLanguage(author index.Author, language string) string {
	return s.forLanguage(language).ProfileValue(author)
}
func (s *Service) NormalizeAuthorID(value string) (string, error) {
	return s.source.NormalizeAuthorID(value)
}

// MetadataServiceName identifies the metadata source that owns this biography's profile.
// An empty name means the biography has an independently editable profile.
func (s *Service) MetadataServiceName() string {
	if source, ok := s.source.(interface{ MetadataServiceName() string }); ok {
		return source.MetadataServiceName()
	}
	return ""
}

// RefreshMetadataBio refreshes the biography from the updated metadata links,
// or clears it when the metadata no longer has a link in the current language.
func (s *Service) RefreshMetadataBio(ctx context.Context, author index.Author, language string) error {
	s = s.forLanguage(language)
	if s.source.ProfileValue(author) == "" {
		_, _, err := s.UpdateBio(ctx, author, "", language)
		return err
	}
	_, _, err := s.RefreshBio(ctx, author, language)
	return err
}

func (s *Service) authorLink(author index.Author) (string, bool) {
	value := author.DataSourceLinks[s.source.LinkKey()]
	id, err := s.source.NormalizeAuthorID(value)
	return value, err == nil && id != "" && id == value
}

// StoredBio returns an indexed biography without network requests. pending reports
// whether Bio must retrieve or translate it; translatedFrom identifies translations.
func (s *Service) StoredBio(author index.Author, language string) (bio, translatedFrom string, pending bool) {
	s = s.forLanguage(language)
	link, ok := s.authorLink(author)
	if !ok {
		return "", "", false
	}
	if author.Bio.Source != link || !s.source.CachedBioValid(author.Bio.Text[author.Bio.Language]) {
		return "", "", true
	}
	if text, ok := author.Bio.Text[language]; ok {
		if language != author.Bio.Language {
			translatedFrom = author.Bio.Language
		}
		return text, translatedFrom, false
	}
	original := author.Bio.Text[author.Bio.Language]
	if original == "" || s.translator == nil {
		return original, "", false
	}
	return "", "", true
}

type bioResult struct{ text, translatedFrom string }

// Bio retrieves or translates a missing biography. Translation failures return the
// original text alongside the error.
func (s *Service) Bio(ctx context.Context, author index.Author, language string) (bio, translatedFrom string, err error) {
	s = s.forLanguage(language)
	if bio, translatedFrom, pending := s.StoredBio(author, language); !pending {
		return bio, translatedFrom, nil
	}
	// Finish storing the lookup even if the reader leaves.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
	defer cancel()
	result, err, _ := s.lookups.Do(author.Slug+"\x00"+language, func() (any, error) {
		text, from, err := s.lookup(ctx, author, language, s.bioStore(author, true), false)
		return bioResult{text, from}, err
	})
	r := result.(bioResult)
	return r.text, r.translatedFrom, err
}

// RefreshBio retrieves a fresh biography, replacing the stored original and translations.
func (s *Service) RefreshBio(ctx context.Context, author index.Author, language string) (bio, translatedFrom string, err error) {
	s = s.forLanguage(language)
	if _, ok := s.authorLink(author); !ok {
		return "", "", nil
	}
	author.Bio = index.AuthorBio{}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
	defer cancel()
	result, err, _ := s.lookups.Do(author.Slug+"\x00refresh\x00"+language, func() (any, error) {
		text, from, err := s.lookup(ctx, author, language, s.bioStore(author, false), true)
		return bioResult{text, from}, err
	})
	r := result.(bioResult)
	return r.text, r.translatedFrom, err
}

type storeFunc func(slug string, bio index.AuthorBio) error

func (s *Service) bioStore(author index.Author, merge bool) storeFunc {
	linkKey := s.source.LinkKey()
	previousLink := author.DataSourceLinks[linkKey]
	return func(slug string, bio index.AuthorBio) error {
		if bio.Source != previousLink {
			return s.index.StoreCanonicalAuthorBio(slug, linkKey, previousLink, bio)
		}
		return s.index.StoreAuthorBioForLink(slug, linkKey, previousLink, bio, merge)
	}
}

// UpdateBio retrieves a new profile's biography, saving its link with the result.
// An empty ID removes the profile and biography.
func (s *Service) UpdateBio(ctx context.Context, author index.Author, id, language string) (string, string, error) {
	s = s.forLanguage(language)
	linkKey := s.source.LinkKey()
	if id == "" {
		if err := s.index.StoreCanonicalAuthorBio(author.Slug, linkKey, author.DataSourceLinks[linkKey], index.AuthorBio{}); err != nil {
			return "", "", fmt.Errorf("clearing %s author link: %w", s.ProfileServiceName(), err)
		}
		return "", "", nil
	}
	normalized, err := s.source.NormalizeAuthorID(id)
	if err != nil {
		return "", "", err
	}
	if normalized == "" || normalized != id {
		return "", "", fmt.Errorf("invalid %s author ID %q", s.ProfileServiceName(), id)
	}
	save := s.bioStore(author, false)
	author.DataSourceLinks = maps.Clone(author.DataSourceLinks)
	if author.DataSourceLinks == nil {
		author.DataSourceLinks = make(map[string]string)
	}
	author.DataSourceLinks[linkKey] = id
	author.Bio = index.AuthorBio{}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
	defer cancel()
	result, err, _ := s.lookups.Do(author.Slug+"\x00update\x00"+id+"\x00"+language, func() (any, error) {
		text, from, err := s.lookup(ctx, author, language, save, true)
		return bioResult{text, from}, err
	})
	bio := result.(bioResult)
	return bio.text, bio.translatedFrom, err
}

func (s *Service) lookup(ctx context.Context, author index.Author, language string, save storeFunc, refresh bool) (string, string, error) {
	link, _ := s.authorLink(author)
	bio := author.Bio
	original := bio.Text[bio.Language]
	if bio.Source != link || !s.source.CachedBioValid(original) {
		text, canonical, err := s.source.RetrieveBio(ctx, link, refresh)
		if err != nil {
			return "", "", fmt.Errorf("retrieving %s author %s: %w", s.ProfileServiceName(), link, err)
		}
		normalized, err := s.source.NormalizeAuthorID(canonical)
		if err != nil {
			return "", "", fmt.Errorf("invalid canonical %s author ID: %w", s.ProfileServiceName(), err)
		}
		if normalized == "" || normalized != canonical {
			return "", "", fmt.Errorf("invalid canonical %s author ID %q", s.ProfileServiceName(), canonical)
		}
		bio = index.AuthorBio{Source: canonical}
		original = text
		if original != "" {
			if source, ok := s.source.(LocalizedSource); ok {
				bio.Language = source.Language()
			} else {
				bio.Language = detectLanguage(html.UnescapeString(bluemonday.StrictPolicy().Sanitize(original)))
			}
		}
		if bio.Language != "" {
			bio.Text = map[string]string{bio.Language: original}
		}
	}
	// Unknown-language originals must not be cached as empty biographies.
	if original == "" || bio.Language == language || (s.translator == nil && bio.Language != "") {
		return original, "", store(save, author.Slug, bio)
	}
	if s.translator == nil {
		return original, "", nil
	}
	from := bio.Language
	if from == "" {
		from = "auto"
	}
	var gender gt.Gender
	switch author.Gender {
	case model.GenderMale, model.GenderTransgenderMale:
		gender = gt.GenderMasculine
	case model.GenderFemale, model.GenderTransgenderFemale:
		gender = gt.GenderFeminine
	}
	translated, detected, err := s.translateBio(ctx, original, from, language, gender)
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

func (s *Service) translateBio(ctx context.Context, text, from, to string, gender gt.Gender) (string, string, error) {
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
				translated, err := s.translator.TranslateWithGender(ctx, strings.TrimSpace(part), from, to, gender)
				if err != nil {
					return "", "", fmt.Errorf("translating text from %s to %s: %w", from, to, err)
				}
				if translated == nil || strings.TrimSpace(translated.Text) == "" {
					return "", "", fmt.Errorf("translating text from %s to %s: empty translation", from, to)
				}
				if from == "auto" {
					detected, _, _ := strings.Cut(strings.TrimSpace(translated.From.Language.Iso), "-")
					from = strings.ToLower(detected)
					if from == "" {
						return "", "", fmt.Errorf("translating text to %s: source language not detected", to)
					}
				}
				part = leading + translated.Text + trailing
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

// CleanHTML retains safe emphasis and converts HTML block boundaries to newlines.
func CleanHTML(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	text = paragraphEnd.ReplaceAllString(text, "\n\n")
	text = htmlLineBreak.ReplaceAllString(text, "\n")
	text = htmlFormatting.Sanitize(text)
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

func detectLanguage(text string) string {
	info := whatlanggo.Detect(text)
	if !info.IsReliable() {
		return ""
	}
	return info.Lang.Iso6391()
}
