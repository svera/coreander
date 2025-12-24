package metadata

import (
	"fmt"
	"sync"

	"github.com/pemistahl/lingua-go"
)

var (
	languageDetector     lingua.LanguageDetector
	detectorOnce         sync.Once
	maxDetectionTextSize = 10000 // Limit text size for faster detection
)

// LanguageDetectionResult represents the result of language detection
type LanguageDetectionResult struct {
	// PrimaryLanguage is the most likely detected language
	PrimaryLanguage string
	// PrimaryLanguageExists indicates if a language was detected
	PrimaryLanguageExists bool
	// ConfidenceValues contains confidence scores for all detected languages
	ConfidenceValues []LanguageConfidence
	// MultipleLanguages contains detected languages in mixed-language texts
	MultipleLanguages []LanguageSection
}

// LanguageConfidence represents a language with its confidence score
type LanguageConfidence struct {
	Language  string
	Confidence float64
}

// LanguageSection represents a section of text in a specific language
type LanguageSection struct {
	Language   string
	StartIndex int
	EndIndex   int
	Text       string
}

// getLanguageDetector returns a singleton language detector instance
func getLanguageDetector() lingua.LanguageDetector {
	detectorOnce.Do(func() {
		// Use low accuracy mode for faster performance and lower memory usage
		// This is sufficient for detecting stop words
		languageDetector = lingua.NewLanguageDetectorBuilder().
			FromAllSpokenLanguages().
			WithLowAccuracyMode().
			Build()
	})
	return languageDetector
}

// DetectLanguage detects the language(s) in an EPUB file
// It extracts text content and uses lingua-go to detect the language(s)
// Uses a text sample for faster detection
func (e EpubReader) DetectLanguage(filename string) (*LanguageDetectionResult, error) {
	// Extract text content from EPUB
	textContent, err := extractTextFromEPUB(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to extract text: %w", err)
	}

	if textContent == "" {
		return nil, fmt.Errorf("no text content found in EPUB")
	}

	// Use a sample of text for faster detection
	textSample := textContent
	if len(textSample) > maxDetectionTextSize {
		textSample = textSample[:maxDetectionTextSize]
	}

	// Get cached language detector
	detector := getLanguageDetector()

	// Detect primary language using text sample
	language, exists := detector.DetectLanguageOf(textSample)

	result := &LanguageDetectionResult{
		PrimaryLanguageExists: exists,
	}

	if exists {
		result.PrimaryLanguage = language.IsoCode639_1().String()
	}

	// Get confidence values for all languages using text sample
	confidenceValues := detector.ComputeLanguageConfidenceValues(textSample)
	result.ConfidenceValues = make([]LanguageConfidence, 0, len(confidenceValues))
	for _, cv := range confidenceValues {
		if cv.Value() > 0.05 { // Only include languages with meaningful confidence
			result.ConfidenceValues = append(result.ConfidenceValues, LanguageConfidence{
				Language:  cv.Language().IsoCode639_1().String(),
				Confidence: float64(cv.Value()),
			})
		}
	}

	// Detect multiple languages in mixed-language texts using text sample
	multipleResults := detector.DetectMultipleLanguagesOf(textSample)
	if len(multipleResults) > 0 {
		result.MultipleLanguages = make([]LanguageSection, 0, len(multipleResults))
		for _, mr := range multipleResults {
			startIdx := mr.StartIndex()
			endIdx := mr.EndIndex()
			if startIdx < len(textContent) && endIdx <= len(textContent) {
				result.MultipleLanguages = append(result.MultipleLanguages, LanguageSection{
					Language:   mr.Language().IsoCode639_1().String(),
					StartIndex: startIdx,
					EndIndex:   endIdx,
					Text:       textContent[startIdx:endIdx],
				})
			}
		}
	}

	return result, nil
}

// DetectLanguageFromText detects the language(s) in a given text string
// This is a utility function that can be used with any text content
// Uses a text sample for faster detection
func DetectLanguageFromText(text string) (*LanguageDetectionResult, error) {
	if text == "" {
		return nil, fmt.Errorf("text is empty")
	}

	// Use a sample of text for faster detection
	textSample := text
	if len(textSample) > maxDetectionTextSize {
		textSample = textSample[:maxDetectionTextSize]
	}

	// Get cached language detector
	detector := getLanguageDetector()

	// Detect primary language using text sample
	language, exists := detector.DetectLanguageOf(textSample)

	result := &LanguageDetectionResult{
		PrimaryLanguageExists: exists,
	}

	if exists {
		result.PrimaryLanguage = language.IsoCode639_1().String()
	}

	// Get confidence values for all languages using text sample
	confidenceValues := detector.ComputeLanguageConfidenceValues(textSample)
	result.ConfidenceValues = make([]LanguageConfidence, 0, len(confidenceValues))
	for _, cv := range confidenceValues {
		if cv.Value() > 0.05 { // Only include languages with meaningful confidence
			result.ConfidenceValues = append(result.ConfidenceValues, LanguageConfidence{
				Language:  cv.Language().IsoCode639_1().String(),
				Confidence: float64(cv.Value()),
			})
		}
	}

	// Detect multiple languages in mixed-language texts using text sample
	multipleResults := detector.DetectMultipleLanguagesOf(textSample)
	if len(multipleResults) > 0 {
		result.MultipleLanguages = make([]LanguageSection, 0, len(multipleResults))
		for _, mr := range multipleResults {
			startIdx := mr.StartIndex()
			endIdx := mr.EndIndex()
			if startIdx < len(text) && endIdx <= len(text) {
				result.MultipleLanguages = append(result.MultipleLanguages, LanguageSection{
					Language:   mr.Language().IsoCode639_1().String(),
					StartIndex: startIdx,
					EndIndex:   endIdx,
					Text:       text[startIdx:endIdx],
				})
			}
		}
	}

	return result, nil
}

