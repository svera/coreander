package metadata

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	textrank "github.com/DavidBelicza/TextRank/v2"
	"github.com/DavidBelicza/TextRank/v2/parse"
	"github.com/DavidBelicza/TextRank/v2/rank"
)

//go:embed stopwords-iso.json
var stopWordsJSON []byte

// TextRankResult represents the result of text ranking analysis
// Phrases and SingleWords are returned as slices from TextRank
type TextRankResult struct {
	Phrases     []rank.Phrase
	SingleWords []rank.SingleWord
}

// RankText performs TextRank analysis on an EPUB file
// It extracts the text content, detects languages in the document,
// and fetches appropriate stop words from all detected languages
func (e EpubReader) RankText(filename string) (*TextRankResult, error) {
	// A zero ratio disables text ranking altogether, rather than meaning "no
	// filtering": since every phrase/word occurs at least once, a ratio of 0
	// would otherwise keep everything, which is rarely what's wanted and
	// wastes the cost of the analysis for nothing.
	if e.MinPhraseOccurrenceRatio == 0 || e.MinWordOccurrenceRatio == 0 {
		return nil, nil
	}

	// Extract text content from EPUB
	textContent, err := extractText(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to extract text: %w", err)
	}

	if textContent == "" {
		return nil, fmt.Errorf("no text content found in EPUB")
	}

	// Always detect languages in the document using full text. Reuses the
	// textContent already extracted above instead of calling DetectLanguage,
	// which would re-extract (and re-sanitize) the whole EPUB from scratch.
	langResult, err := DetectLanguageFromText(textContent)
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

	// Collect all detected languages, always including English so its stop
	// words are filtered even when English isn't one of the document's
	// detected languages.
	detectedLanguages := map[string]bool{"en": true}

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
	if len(stopWords) > 0 {
		// TextRank's LanguageDefault only filters stop words for the
		// currently active key, so English and the document's detected
		// languages' words must be merged into this single key rather than
		// loaded under separate per-language keys.
		language.SetWords("multi", stopWords)
		language.SetActiveLanguage("multi")
	}

	// Use a rule that also treats punctuation (em dashes, ellipses, quotation
	// marks, etc.) as a word separator, so it never becomes part of a single
	// word or phrase in the first place.
	rule := newPunctuationAwareRule()

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

	// Filter out phrases whose occurrence count is far from the most frequent
	// phrase's: Weight is normalized against this document's own min/max
	// occurrence counts, so it can't tell a genuinely rare phrase from one
	// that just occurs once in a document where most phrases do.
	phrases = filterPhrasesByOccurrenceRatio(phrases, e.MinPhraseOccurrenceRatio)

	// Filter out single words that are far less frequent than the most
	// frequent word, for the same reason phrases are filtered above.
	singleWords = filterSingleWordsByOccurrenceRatio(singleWords, e.MinWordOccurrenceRatio)

	return &TextRankResult{
		Phrases:     phrases,
		SingleWords: singleWords,
	}, nil
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

// punctuationAwareRule extends the library's default word/sentence
// separators with general punctuation (em dashes, en dashes, ellipses,
// quotation marks, etc.), which the default rule leaves attached to
// whatever word or phrase they're adjacent to.
type punctuationAwareRule struct {
	*parse.RuleDefault
}

func newPunctuationAwareRule() *punctuationAwareRule {
	return &punctuationAwareRule{parse.NewRule()}
}

// IsWordSeparator reports whether rune should split words, treating any
// punctuation other than apostrophes and hyphens (which can be part of
// legitimate words, e.g. "don't", "well-known") as a separator too.
func (r *punctuationAwareRule) IsWordSeparator(rune rune) bool {
	if r.RuleDefault.IsWordSeparator(rune) {
		return true
	}
	if unicode.IsSpace(rune) {
		// The default rule only treats a literal space and newline as
		// separators, leaving other whitespace (tabs, carriage returns, etc.
		// left over from indentation in the source HTML) attached to words.
		return true
	}
	return unicode.IsPunct(rune) && rune != '\'' && rune != '-'
}

// filterPhrasesByOccurrenceRatio removes phrases whose occurrence count is
// below ratio of the most frequent phrase's occurrence count. Weight alone
// can't be used for this: it's normalized against the document's own
// min/max occurrence counts, not an absolute measure of relevance.
func filterPhrasesByOccurrenceRatio(phrases []rank.Phrase, ratio float64) []rank.Phrase {
	if len(phrases) == 0 {
		return phrases
	}

	maxQty := 0
	for _, phrase := range phrases {
		if phrase.Qty > maxQty {
			maxQty = phrase.Qty
		}
	}
	threshold := float64(maxQty) * ratio

	filtered := make([]rank.Phrase, 0, len(phrases))
	for _, phrase := range phrases {
		if float64(phrase.Qty) >= threshold {
			filtered = append(filtered, phrase)
		}
	}

	return filtered
}

// filterSingleWordsByOccurrenceRatio removes single words whose occurrence
// count is below ratio of the most frequent word's occurrence count.
// See filterPhrasesByOccurrenceRatio for why Weight alone isn't enough.
func filterSingleWordsByOccurrenceRatio(words []rank.SingleWord, ratio float64) []rank.SingleWord {
	if len(words) == 0 {
		return words
	}

	maxQty := 0
	for _, word := range words {
		if word.Qty > maxQty {
			maxQty = word.Qty
		}
	}
	threshold := float64(maxQty) * ratio

	filtered := make([]rank.SingleWord, 0, len(words))
	for _, word := range words {
		if float64(word.Qty) >= threshold {
			filtered = append(filtered, word)
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
