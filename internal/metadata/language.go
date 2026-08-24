package metadata

import (
	"fmt"

	"github.com/abadojack/whatlanggo"
)

const maxDetectionTextSize = 10000 // Limit text size for faster detection

// LanguageDetectionResult represents the result of language detection
type LanguageDetectionResult struct {
	// PrimaryLanguage is the detected language's ISO 639-1 code
	PrimaryLanguage string
	// PrimaryLanguageExists indicates if a language was detected
	PrimaryLanguageExists bool
}

// DetectLanguageFromText detects the primary language of a given text string.
// This is a utility function that can be used with any text content.
// Uses a text sample for faster detection.
func DetectLanguageFromText(text string) (*LanguageDetectionResult, error) {
	if text == "" {
		return nil, fmt.Errorf("text is empty")
	}

	// Use a sample of text for faster detection
	textSample := text
	if len(textSample) > maxDetectionTextSize {
		textSample = textSample[:maxDetectionTextSize]
	}

	info := whatlanggo.Detect(textSample)
	code := info.Lang.Iso6391()

	return &LanguageDetectionResult{
		PrimaryLanguage:       code,
		PrimaryLanguageExists: code != "",
	}, nil
}
