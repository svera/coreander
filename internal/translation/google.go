// Package translation translates texts between languages using an online service.
package translation

import (
	"context"
	"fmt"
	"strings"

	"gopkg.gilang.dev/translator/v2/googletranslate"
)

// AutoDetect asks the service to detect the source language.
const AutoDetect = "auto"

// Gender selects a matching translation when the service provides gendered alternatives.
type Gender uint8

const (
	GenderUnknown Gender = iota
	GenderMasculine
	GenderFeminine
)

type client interface {
	TranslateWithGender(ctx context.Context, text, from, to string, gender googletranslate.Gender) (*googletranslate.Translated, error)
}

// Google translates using the free Google Translate web endpoint.
type Google struct {
	client client
}

func NewGoogle() *Google {
	return &Google{client: googletranslate.New()}
}

// Translate translates text from the from language (or AutoDetect) to the to language, both ISO 639-1 codes.
// It returns the translation and the source language, which is the detected one when from is AutoDetect.
// Line breaks are preserved.
func (g *Google) Translate(ctx context.Context, text, from, to string, gender Gender) (string, string, error) {
	var preference googletranslate.Gender
	switch gender {
	case GenderMasculine:
		preference = googletranslate.GenderMasculine
	case GenderFeminine:
		preference = googletranslate.GenderFeminine
	}
	if strings.TrimSpace(text) == "" {
		return text, from, nil
	}
	result, err := g.client.TranslateWithGender(ctx, text, from, to, preference)
	if err != nil {
		return "", "", fmt.Errorf("translating text from %s to %s: %w", from, to, err)
	}
	if result == nil || strings.TrimSpace(result.Text) == "" {
		return "", "", fmt.Errorf("translating text from %s to %s: empty translation", from, to)
	}
	if from == AutoDetect {
		if from = NormalizeLanguage(result.From.Language.Iso); from == "" {
			return "", "", fmt.Errorf("translating text to %s: source language not detected", to)
		}
	}
	return result.Text, from, nil
}

// NormalizeLanguage reduces language tags such as "zh-CN" to lowercase ISO 639-1 codes.
func NormalizeLanguage(code string) string {
	code, _, _ = strings.Cut(strings.TrimSpace(code), "-")
	return strings.ToLower(code)
}
