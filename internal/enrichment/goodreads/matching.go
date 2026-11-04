package goodreads

import (
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/index"
	"golang.org/x/text/unicode/norm"
)

// Strip named series annotations and omnibus ranges, not standalone volume numbers.
var goodReadsSeriesSuffix = regexp.MustCompile(`\s*\((?:[^()]+,\s*#[0-9]+(?:\.[0-9]+)?|#[0-9]+\s*[-–]\s*#?[0-9]+)\)\s*$`)

var goodReadsSubtitleSeparator = regexp.MustCompile(`:\s*|\.\s+|\s+[-–—]\s+`)

var goodReadsParentheticalTitle = regexp.MustCompile(`\s+\([\p{L}][^()]*\)\s*$`)

var goodReadsBookPath = regexp.MustCompile(`^/book/show/([1-9][0-9]*)(?:[.-][A-Za-z0-9._~-]+)?/?$`)

func normalizeGoodReadsName(value string) string {
	var result strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(value)) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case unicode.IsLetter(r), unicode.IsNumber(r):
			result.WriteRune(r)
		default:
			result.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(result.String()), " ")
}

func goodReadsAuthorNameMatches(name, candidate string) bool {
	name = normalizeGoodReadsName(name)
	candidate = normalizeGoodReadsName(candidate)
	if name == "" || candidate == "" {
		return false
	}
	if strings.ReplaceAll(name, " ", "") == strings.ReplaceAll(candidate, " ", "") {
		return true
	}
	short, long := strings.Fields(name), strings.Fields(candidate)
	if len(short) > len(long) {
		short, long = long, short
	}
	if len(short) < 2 || short[0] != long[0] || short[len(short)-1] != long[len(long)-1] {
		return false
	}
	// Allow omitted middle names, but not conflicting ones.
	next := 1
	for _, part := range long[1 : len(long)-1] {
		if next < len(short)-1 && part == short[next] {
			next++
		}
	}
	return next == len(short)-1
}

func goodReadsAuthor(document index.Document, matchedName string) (string, string) {
	nameKey := strings.ReplaceAll(normalizeGoodReadsName(matchedName), " ", "")
	if nameKey == "" {
		return "", ""
	}
	for i, name := range document.Authors {
		if i < len(document.AuthorsSlugs) && strings.ReplaceAll(normalizeGoodReadsName(name), " ", "") == nameKey {
			return document.AuthorsSlugs[i], name
		}
	}
	var slug, authorName string
	for i, name := range document.Authors {
		if i < len(document.AuthorsSlugs) && goodReadsAuthorNameMatches(name, matchedName) {
			if slug != "" {
				return "", ""
			}
			slug, authorName = document.AuthorsSlugs[i], name
		}
	}
	return slug, authorName
}

func goodReadsTitle(value string) string {
	return normalizeGoodReadsName(goodReadsSeriesSuffix.ReplaceAllString(value, ""))
}

func goodReadsMainTitle(value string) string {
	value = goodReadsSeriesSuffix.ReplaceAllString(value, "")
	if separator := goodReadsSubtitleSeparator.FindStringIndex(value); separator != nil {
		value = value[:separator[0]]
	}
	return normalizeGoodReadsName(value)
}

func goodReadsTitleMatchScore(candidate, title, mainTitle string) int {
	candidateTitle := goodReadsTitle(candidate)
	if candidateTitle == "" {
		return 0
	}
	if candidateTitle == title {
		return 3
	}
	if goodReadsMainTitle(candidate) == title || mainTitle == candidateTitle ||
		goodReadsTitle(goodReadsParentheticalTitle.ReplaceAllString(candidate, "")) == title {
		return 1
	}
	return 0
}

func preferGoodReadsBook(candidate, best biblioreads.SearchResult, score, bestScore int) bool {
	if bestScore == 0 {
		return true
	}
	if score == bestScore {
		count, bestCount := goodReadsRatingCount(candidate.RatingCount), goodReadsRatingCount(best.RatingCount)
		if count != bestCount {
			return count > bestCount
		}
		id, bestID := goodReadsBookID(candidate.BookURL), goodReadsBookID(best.BookURL)
		if id > 0 && bestID > 0 && id != bestID {
			return id < bestID
		}
	}
	hasRating, bestHasRating := hasGoodReadsRating(candidate.Rating), hasGoodReadsRating(best.Rating)
	if hasRating != bestHasRating {
		return hasRating
	}
	return score > bestScore
}

func goodReadsRatingCount(value string) uint64 {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	count, err := strconv.ParseUint(strings.ReplaceAll(strings.TrimSpace(value), ",", ""), 10, 64)
	if err != nil {
		log.Printf("Invalid GoodReads rating count: %q", value)
		return 0
	}
	return count
}

func goodReadsBookID(bookURL string) uint64 {
	match := goodReadsBookPath.FindStringSubmatch(bookURL)
	if match == nil {
		return 0
	}
	id, err := strconv.ParseUint(match[1], 10, 64)
	if err != nil {
		log.Printf("Invalid GoodReads book ID in URL: %q", bookURL)
		return 0
	}
	return id
}

func matchingGoodReadsBook(document index.Document, results []biblioreads.SearchResult) (biblioreads.SearchResult, bool) {
	title := goodReadsTitle(document.Title)
	if title == "" {
		return biblioreads.SearchResult{}, false
	}
	mainTitle := goodReadsMainTitle(document.Title)
	var best biblioreads.SearchResult
	bestScore := 0
	for _, result := range results {
		if result.BookURL == "" {
			continue
		}
		score := goodReadsTitleMatchScore(result.Title, title, mainTitle)
		if score == 0 {
			continue
		}
		for _, author := range document.Authors {
			if goodReadsAuthorNameMatches(author, result.Author) {
				if preferGoodReadsBook(result, best, score, bestScore) {
					best, bestScore = result, score
				}
				break
			}
		}
	}
	return best, bestScore > 0
}

func hasGoodReadsRating(rating string) bool {
	value, err := strconv.ParseFloat(rating, 64)
	return err == nil && value > 0 && value <= 5 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

// goodReadsQuery returns the Goodreads search string for document: its title
// followed by its first author, if any.
func goodReadsQuery(document index.Document) string {
	q := strings.TrimSpace(document.Title)
	if len(document.Authors) > 0 {
		q = strings.TrimSpace(q + " " + document.Authors[0])
	}
	return q
}

func parseGoodReadsRating(rating, documentID string) float64 {
	if rating == "" {
		return 0
	}
	value, err := strconv.ParseFloat(rating, 64)
	if err != nil || value < 0 || value > 5 || math.IsNaN(value) || math.IsInf(value, 0) {
		log.Printf("Invalid GoodReads rating for document %s: %q", documentID, rating)
		return 0
	}
	return value
}
