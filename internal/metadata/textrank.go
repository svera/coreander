package metadata

import (
	"archive/zip"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"

	textrank "github.com/DavidBelicza/TextRank/v2"
	"github.com/DavidBelicza/TextRank/v2/rank"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/microcosm-cc/bluemonday"
)

//go:embed stopwords-iso.json
var stopWordsJSON []byte

// TextRankResult represents the result of text ranking analysis
// Phrases, Sentences, and SingleWords are returned as slices from TextRank
type TextRankResult struct {
	Phrases     []rank.Phrase
	Sentences   []rank.Sentence
	SingleWords []rank.SingleWord
}

// RankText performs TextRank analysis on an EPUB file
// It extracts the text content, detects languages in the document,
// and fetches appropriate stop words from all detected languages
func (e EpubReader) RankText(filename string) (*TextRankResult, error) {
	// Extract text content from EPUB
	textContent, err := extractTextFromEPUB(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to extract text: %w", err)
	}

	if textContent == "" {
		return nil, fmt.Errorf("no text content found in EPUB")
	}

	// Always detect languages in the document using full text
	langResult, err := e.DetectLanguage(filename)
	if err != nil {
		// If language detection fails, fall back to metadata language
		meta, metaErr := e.GetMetadataFromFile(filename)
		if metaErr != nil {
			return nil, fmt.Errorf("failed to detect language and get metadata: %w", err)
		}
		metadataLang := ""
		if len(meta.Language) > 0 {
			metadataLang = meta.Language[0]
			// Normalize language code (e.g., "en-US" -> "en")
			if idx := strings.Index(metadataLang, "-"); idx != -1 {
				metadataLang = strings.ToLower(metadataLang[:idx])
			} else {
				metadataLang = strings.ToLower(metadataLang)
			}
		}
		if metadataLang != "" {
			langResult = &LanguageDetectionResult{
				PrimaryLanguage:       metadataLang,
				PrimaryLanguageExists: true,
				ConfidenceValues: []LanguageConfidence{
					{
						Language:   metadataLang,
						Confidence: 1.0,
					},
				},
			}
		} else {
			return nil, fmt.Errorf("failed to detect language and no metadata language available: %w", err)
		}
	}

	// Collect all detected languages
	detectedLanguages := make(map[string]bool)

	// Add primary language if detected
	if langResult.PrimaryLanguageExists && langResult.PrimaryLanguage != "" {
		detectedLanguages[langResult.PrimaryLanguage] = true
	}

	// Add languages from confidence values (with confidence > 0.1)
	for _, cv := range langResult.ConfidenceValues {
		if cv.Confidence > 0.1 && cv.Language != "" {
			detectedLanguages[cv.Language] = true
		}
	}

	// Add languages from multiple language sections
	for _, section := range langResult.MultipleLanguages {
		if section.Language != "" {
			detectedLanguages[section.Language] = true
		}
	}

	// Convert map to slice of language codes
	langCodes := make([]string, 0, len(detectedLanguages))
	for langCode := range detectedLanguages {
		langCodes = append(langCodes, langCode)
	}

	// Fetch stop words for all detected languages
	stopWords, err := fetchStopWordsForLanguages(langCodes)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch stop words: %w", err)
	}

	// Create TextRank instance
	tr := textrank.NewTextRank()

	// Create language with stop words
	language := textrank.NewDefaultLanguage()
	if len(stopWords) > 0 && len(langCodes) > 0 {
		// Use the primary language as the active language, or first detected language
		activeLang := langResult.PrimaryLanguage
		if activeLang == "" && len(langCodes) > 0 {
			activeLang = langCodes[0]
		}
		if activeLang != "" {
			language.SetWords(activeLang, stopWords)
			language.SetActiveLanguage(activeLang)
		}
	}

	// Use default rule for parsing
	rule := textrank.NewDefaultRule()

	// Use default algorithm for ranking
	algorithm := textrank.NewDefaultAlgorithm()

	// Populate TextRank with text content
	tr.Populate(textContent, language, rule)

	// Run ranking
	tr.Ranking(algorithm)

	// Extract results
	phrases := textrank.FindPhrases(tr)
	singleWords := textrank.FindSingleWords(tr)

	// Fix phrase order using word connection data
	rankData := tr.GetRankData()
	phrases = fixPhraseOrder(phrases, rankData)

	// Filter out phrases containing punctuation
	phrases = filterPhrasesWithPunctuation(phrases)

	return &TextRankResult{
		Phrases:     phrases,
		Sentences:   nil, // Sentences are not computed
		SingleWords: singleWords,
	}, nil
}

// extractTextFromEPUB extracts all text content from an EPUB file
// Similar to the words() function but returns the full text instead of counting
func extractTextFromEPUB(documentFullPath string) (string, error) {
	r, err := zip.OpenReader(documentFullPath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	var textParts []string
	p := bluemonday.StrictPolicy()

	for _, f := range r.File {
		isContent, err := doublestar.PathMatch("O*PS/**/*.*htm*", f.Name)
		if err != nil {
			return "", err
		}
		if !isContent {
			continue
		}

		// Skip navigation and table of contents files
		baseName := strings.ToLower(filepath.Base(f.Name))
		if baseName == "nav.xhtml" || baseName == "toc.xhtml" {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return "", err
		}

		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return "", err
		}

		// Sanitize HTML to get plain text
		text := p.Sanitize(string(content))
		if strings.TrimSpace(text) != "" {
			textParts = append(textParts, text)
		}
	}

	return strings.Join(textParts, "\n\n"), nil
}

// fixPhraseOrder corrects the order of words in phrases based on their actual connection data
// TextRank may store phrases with Left/Right based on word IDs rather than text position
// We use connection occurrence counts to determine the correct order
func fixPhraseOrder(phrases []rank.Phrase, rankData *rank.Rank) []rank.Phrase {
	if rankData == nil || rankData.Words == nil {
		return phrases
	}

	fixedPhrases := make([]rank.Phrase, len(phrases))
	for i, p := range phrases {
		fixedPhrases[i] = p

		leftWord, leftExists := rankData.Words[p.LeftID]
		rightWord, rightExists := rankData.Words[p.RightID]

		if !leftExists || !rightExists {
			continue
		}

		// Get occurrence counts for both possible orders
		leftToRightCount := 0
		if count, exists := leftWord.ConnectionRight[p.RightID]; exists {
			leftToRightCount = count
		}

		rightToLeftCount := 0
		if count, exists := rightWord.ConnectionRight[p.LeftID]; exists {
			rightToLeftCount = count
		}

		// Determine correct order based on occurrence counts
		shouldSwap := false

		if rightToLeftCount > leftToRightCount {
			// Right-to-left order has more occurrences
			shouldSwap = true
		} else if leftToRightCount == 0 && rightToLeftCount > 0 {
			// Only right-to-left order exists
			shouldSwap = true
		} else if leftToRightCount == rightToLeftCount && leftToRightCount > 0 {
			// Both orders have equal counts, check sentence positions as tiebreaker
			shouldSwap = checkSentenceOrder(leftWord, rightWord, rankData, p.Left, p.Right)
		}

		if shouldSwap {
			fixedPhrases[i].LeftID, fixedPhrases[i].RightID = fixedPhrases[i].RightID, fixedPhrases[i].LeftID
			fixedPhrases[i].Left, fixedPhrases[i].Right = fixedPhrases[i].Right, fixedPhrases[i].Left
		}
	}

	return fixedPhrases
}

// checkSentenceOrder checks which word appears first in shared sentences
// Returns true if right word appears before left word in most sentences
func checkSentenceOrder(leftWord, rightWord *rank.Word, rankData *rank.Rank, leftToken, rightToken string) bool {
	if rankData.SentenceMap == nil {
		return false
	}

	// Find sentences that contain both words
	leftFirstCount := 0
	rightFirstCount := 0

	for _, sentenceID := range leftWord.SentenceIDs {
		// Check if this sentence also contains the right word
		containsRight := false
		for _, rightSentenceID := range rightWord.SentenceIDs {
			if sentenceID == rightSentenceID {
				containsRight = true
				break
			}
		}

		if !containsRight {
			continue
		}

		// Get the sentence text
		sentence, exists := rankData.SentenceMap[sentenceID]
		if !exists {
			continue
		}

		// Find positions of both words in the sentence (case-insensitive)
		sentenceLower := strings.ToLower(sentence)
		leftTokenLower := strings.ToLower(leftToken)
		rightTokenLower := strings.ToLower(rightToken)

		leftPos := strings.Index(sentenceLower, leftTokenLower)
		rightPos := strings.Index(sentenceLower, rightTokenLower)

		if leftPos == -1 || rightPos == -1 {
			continue
		}

		// Count which appears first
		if leftPos < rightPos {
			leftFirstCount++
		} else {
			rightFirstCount++
		}
	}

	// If right word appears first more often, we should swap
	return rightFirstCount > leftFirstCount
}

// filterPhrasesWithPunctuation removes phrases that contain punctuation characters
// This filters out phrases like "word — word" or "word-word" that contain dashes, em dashes, etc.
func filterPhrasesWithPunctuation(phrases []rank.Phrase) []rank.Phrase {
	filtered := make([]rank.Phrase, 0, len(phrases))

	for _, phrase := range phrases {
		// Combine left and right words to check the full phrase
		fullPhrase := phrase.Left + " " + phrase.Right

		// Check if phrase contains punctuation characters
		hasPunctuation := false
		for _, r := range fullPhrase {
			// Check for various punctuation marks including em dash, en dash, etc.
			if unicode.IsPunct(r) && r != '\'' && r != '-' {
				// Allow apostrophes and regular hyphens, but filter others
				hasPunctuation = true
				break
			}
			// Also check for specific dash characters
			if r == '—' || r == '–' || r == '…' {
				hasPunctuation = true
				break
			}
		}

		// Only include phrases without punctuation
		if !hasPunctuation {
			filtered = append(filtered, phrase)
		}
	}

	return filtered
}

// fetchStopWordsForLanguages reads stop words for multiple languages from the embedded stopwords-iso JSON file
// Combines stop words from all provided languages, removing duplicates
// Returns an empty slice if no languages are provided or none are found
func fetchStopWordsForLanguages(langCodes []string) ([]string, error) {
	if len(langCodes) == 0 {
		return []string{}, nil
	}

	// Parse embedded JSON data
	var stopWordsMap map[string][]string
	if err := json.Unmarshal(stopWordsJSON, &stopWordsMap); err != nil {
		return nil, fmt.Errorf("failed to parse stop words JSON: %w", err)
	}

	// Combine stop words from all languages, removing duplicates
	stopWordsSet := make(map[string]bool)
	var combinedStopWords []string

	// Process each language code
	for _, langCode := range langCodes {
		// Normalize language code (e.g., "en-US" -> "en", "es-ES" -> "es")
		normalizedLang := strings.ToLower(langCode)
		if idx := strings.Index(normalizedLang, "-"); idx != -1 {
			normalizedLang = normalizedLang[:idx]
		}

		// Get stop words for this language
		langStopWords, exists := stopWordsMap[normalizedLang]
		if !exists {
			continue
		}

		// Add stop words, avoiding duplicates
		for _, word := range langStopWords {
			wordLower := strings.ToLower(word)
			if !stopWordsSet[wordLower] {
				stopWordsSet[wordLower] = true
				combinedStopWords = append(combinedStopWords, word)
			}
		}
	}

	return combinedStopWords, nil
}

// fetchStopWords reads stop words for a given language from the embedded stopwords-iso JSON file
// Deprecated: Use fetchStopWordsForLanguages instead for multiple languages
func fetchStopWords(lang string) ([]string, error) {
	langCodes := []string{}
	if lang != "" {
		langCodes = append(langCodes, lang)
	}
	return fetchStopWordsForLanguages(langCodes)
}
