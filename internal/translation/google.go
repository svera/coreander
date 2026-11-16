// Package translation translates texts between languages using an online service.
package translation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"gopkg.gilang.dev/translator/v2/googletranslate"
)

// AutoDetect asks the service to detect the source language.
const AutoDetect = "auto"

// maxChunkSize keeps requests below the service limit of 5000 characters per request.
const maxChunkSize = 4500

type client interface {
	Translate(ctx context.Context, text, from, to string) (*googletranslate.Translated, error)
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
func (g *Google) Translate(ctx context.Context, text, from, to string) (string, string, error) {
	var translated []string
	for _, chunk := range chunks(text, maxChunkSize) {
		if strings.TrimSpace(chunk) == "" {
			translated = append(translated, chunk)
			continue
		}
		result, err := g.client.Translate(ctx, escape(chunk), from, to)
		if err != nil {
			return "", "", fmt.Errorf("translating text from %s to %s: %w", from, to, err)
		}
		if from == AutoDetect {
			if from = NormalizeLanguage(result.From.Language.Iso); from == "" {
				return "", "", fmt.Errorf("translating text to %s: source language not detected", to)
			}
		}
		translated = append(translated, trimLines(result.Text))
	}
	return strings.Join(translated, "\n"), from, nil
}

// NormalizeLanguage reduces language tags such as "zh-CN" to lowercase ISO 639-1 codes.
func NormalizeLanguage(code string) string {
	code, _, _ = strings.Cut(strings.TrimSpace(code), "-")
	return strings.ToLower(code)
}

// escape works around the client embedding the text in a JSON string without escaping it,
// which makes requests with quotes, backslashes or line breaks fail.
func escape(text string) string {
	encoded, _ := json.Marshal(text)
	return string(encoded[1 : len(encoded)-1])
}

// trimLines removes the spaces the service adds around line breaks.
func trimLines(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.Join(lines, "\n")
}

// chunks splits text at line breaks, or at sentence and word boundaries for longer lines,
// so each chunk has at most size bytes. Joining the chunks with line breaks restores the text
// layout.
func chunks(text string, size int) []string {
	var (
		result  []string
		current []string
	)
	length := -1
	for line := range strings.SplitSeq(text, "\n") {
		for _, part := range splitLine(line, size) {
			if len(current) > 0 && length+1+len(part) > size {
				result = append(result, strings.Join(current, "\n"))
				current, length = nil, -1
			}
			current = append(current, part)
			length += 1 + len(part)
		}
	}
	return append(result, strings.Join(current, "\n"))
}

func splitLine(line string, size int) []string {
	var parts []string
	for len(line) > size {
		cut := strings.LastIndex(line[:size], ". ")
		if cut > 0 {
			cut++
		} else if cut = strings.LastIndex(line[:size], " "); cut <= 0 {
			cut = size
			for cut > 0 && !utf8.RuneStart(line[cut]) {
				cut--
			}
		}
		parts = append(parts, strings.TrimSpace(line[:cut]))
		line = strings.TrimSpace(line[cut:])
	}
	return append(parts, line)
}
