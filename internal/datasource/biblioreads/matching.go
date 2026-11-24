package biblioreads

import (
	"log"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/svera/coreander/v5/internal/index"
	"golang.org/x/text/unicode/norm"
)

// Strip named series annotations and omnibus ranges, not standalone volume numbers.
var seriesSuffix = regexp.MustCompile(`\s*\((?:([^()]+),\s*#([0-9]+(?:\.[0-9]+)?)|#[0-9]+\s*[-–]\s*#?[0-9]+)\)\s*$`)

var subtitleSeparator = regexp.MustCompile(`:\s*|\.\s+|\s+[-–—]\s+`)

var parentheticalTitle = regexp.MustCompile(`\s+\(([^()]*)\)\s*$`)
var standaloneVolume = regexp.MustCompile(`(?i)^(?:[0-9]+|[ivxlcdm]+)$`)

var goodreadsBookPath = regexp.MustCompile(`^/book/show/([1-9][0-9]*)(?:[.-][A-Za-z0-9._~-]+)?/?$`)

func normalizeName(value string) string {
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

func authorNameMatches(name, candidate string) bool {
	name = normalizeName(name)
	candidate = normalizeName(candidate)
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

func matchedAuthor(document index.Document, matchedName string) (string, string) {
	nameKey := strings.ReplaceAll(normalizeName(matchedName), " ", "")
	if nameKey == "" {
		return "", ""
	}
	for i, name := range document.Authors {
		if i < len(document.AuthorsSlugs) && strings.ReplaceAll(normalizeName(name), " ", "") == nameKey {
			return document.AuthorsSlugs[i], name
		}
	}
	var slug, authorName string
	for i, name := range document.Authors {
		if i < len(document.AuthorsSlugs) && authorNameMatches(name, matchedName) {
			if slug != "" {
				return "", ""
			}
			slug, authorName = document.AuthorsSlugs[i], name
		}
	}
	return slug, authorName
}

func normalizedTitle(value string) string {
	return normalizeName(seriesSuffix.ReplaceAllString(value, ""))
}

func withoutParenthetical(value string) string {
	match := parentheticalTitle.FindStringSubmatchIndex(value)
	if match == nil {
		return value
	}
	annotation := value[match[2]:match[3]]
	normalized := normalizeName(annotation)
	if normalized == "" || strings.Contains(annotation, "#") || standaloneVolume.MatchString(normalized) {
		return value
	}
	for _, word := range strings.Fields(normalized) {
		switch word {
		case "vol", "volume", "volumen", "tomo", "tome", "part", "parte", "book", "libro", "livre":
			return value
		}
	}
	return strings.TrimSpace(value[:match[0]])
}

func mainTitle(value string) string {
	value = seriesSuffix.ReplaceAllString(value, "")
	if separator := subtitleSeparator.FindStringIndex(value); separator != nil {
		value = value[:separator[0]]
	}
	return normalizeName(value)
}

func titleMatchScore(candidate, title, documentMainTitle, documentWithoutParenthetical string) int {
	candidateTitle := normalizedTitle(candidate)
	if candidateTitle == "" {
		return 0
	}
	if candidateTitle == title {
		return 3
	}
	if mainTitle(candidate) == title || documentMainTitle == candidateTitle ||
		normalizedTitle(withoutParenthetical(candidate)) == title ||
		(documentWithoutParenthetical != "" && documentWithoutParenthetical == candidateTitle) {
		return 1
	}
	return 0
}

func preferBook(candidate, best SearchResult, score, bestScore int) bool {
	if bestScore == 0 {
		return true
	}
	if score == bestScore {
		count, bestCount := ratingCount(candidate.RatingCount), ratingCount(best.RatingCount)
		if count != bestCount {
			return count > bestCount
		}
		id, bestID := goodreadsBookID(candidate.BookURL), goodreadsBookID(best.BookURL)
		if id > 0 && bestID > 0 && id != bestID {
			return id < bestID
		}
	}
	hasRating, bestHasRating := hasRating(candidate.Rating), hasRating(best.Rating)
	if hasRating != bestHasRating {
		return hasRating
	}
	return score > bestScore
}

func ratingCount(value string) uint64 {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	count, err := strconv.ParseUint(strings.ReplaceAll(strings.TrimSpace(value), ",", ""), 10, 64)
	if err != nil {
		log.Printf("Invalid BiblioReads rating count: %q", value)
		return 0
	}
	return count
}

func goodreadsBookID(bookURL string) uint64 {
	match := goodreadsBookPath.FindStringSubmatch(bookURL)
	if match == nil {
		return 0
	}
	id, err := strconv.ParseUint(match[1], 10, 64)
	if err != nil {
		log.Printf("Invalid Goodreads book ID in URL: %q", bookURL)
		return 0
	}
	return id
}

func matchingBook(document index.Document, results []SearchResult) (SearchResult, bool) {
	title := normalizedTitle(document.Title)
	if title == "" {
		return SearchResult{}, false
	}
	mainTitle := mainTitle(document.Title)
	documentWithoutParenthetical := normalizedTitle(withoutParenthetical(document.Title))
	matchScore := func(candidate string) int {
		score := titleMatchScore(candidate, title, mainTitle, documentWithoutParenthetical)
		if score != 0 || document.SeriesIndex <= 0 || math.IsNaN(document.SeriesIndex) || math.IsInf(document.SeriesIndex, 0) {
			return score
		}
		series := normalizeName(document.Series)
		annotation := seriesSuffix.FindStringSubmatch(candidate)
		if series == "" || annotation == nil || normalizeName(annotation[1]) != series {
			return 0
		}
		position, err := strconv.ParseFloat(annotation[2], 64)
		if err != nil {
			log.Printf("Invalid BiblioReads series position in title: %q", candidate)
			return 0
		}
		if position == document.SeriesIndex {
			return 1
		}
		return 0
	}
	var best SearchResult
	bestScore := 0
	candidates := make([]SearchResult, 0, len(results))
	byURL := make(map[string]int)
	for _, result := range results {
		if result.BookURL == "" || matchScore(result.Title) == 0 {
			continue
		}
		if !slices.ContainsFunc(document.Authors, func(author string) bool {
			return authorNameMatches(author, result.Author)
		}) {
			continue
		}
		if previous, ok := byURL[result.BookURL]; ok {
			existing := candidates[previous]
			score := matchScore(result.Title)
			existingScore := matchScore(existing.Title)
			if preferBook(result, existing, score, existingScore) {
				candidates[previous] = result
			}
			continue
		}
		byURL[result.BookURL] = len(candidates)
		candidates = append(candidates, result)
	}
	for _, result := range candidates {
		score := matchScore(result.Title)
		if preferBook(result, best, score, bestScore) {
			best, bestScore = result, score
		}
	}
	return best, bestScore > 0
}

func hasRating(rating string) bool {
	value, err := strconv.ParseFloat(rating, 64)
	return err == nil && value > 0 && value <= 5 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

// searchQuery returns the  search string for document: its title
// followed by its first author, if any.
func searchQuery(document index.Document) string {
	q := strings.TrimSpace(document.Title)
	if len(document.Authors) > 0 {
		q = strings.TrimSpace(q + " " + document.Authors[0])
	}
	return q
}

func parseRating(rating, documentID string) float64 {
	if rating == "" {
		return 0
	}
	value, err := strconv.ParseFloat(rating, 64)
	if err != nil || value < 0 || value > 5 || math.IsNaN(value) || math.IsInf(value, 0) {
		log.Printf("Invalid BiblioReads rating for document %s: %q", documentID, rating)
		return 0
	}
	return value
}
