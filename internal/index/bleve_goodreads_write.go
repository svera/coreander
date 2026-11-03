package index

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/blevesearch/bleve/v2"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"golang.org/x/text/unicode/norm"
)

const (
	goodReadsEnrichWorkers  = documentEnrichWorkers
	goodReadsRequestTimeout = 2 * time.Minute
)

// GoodReadsSource searches books on Goodreads; implemented by *biblioreads.Client.
type GoodReadsSource interface {
	SearchBooks(ctx context.Context, query string) (biblioreads.SearchBooksResponse, error)
}

// GoodReadsEnricher adapts Goodreads matching to the document enrichment pipeline.
type GoodReadsEnricher struct {
	source GoodReadsSource
	slots  chan struct{}
}

func NewGoodReadsEnricher(source GoodReadsSource) *GoodReadsEnricher {
	return &GoodReadsEnricher{source: source, slots: make(chan struct{}, goodReadsEnrichWorkers)}
}

func (*GoodReadsEnricher) Source() string { return biblioreads.Source }

func (*GoodReadsEnricher) NeedsEnrichment(document Document) (bool, error) {
	data, err := DataSourceData[biblioreads.DocumentData](document, biblioreads.Source)
	return !data.Enriched, err
}

func (p *GoodReadsEnricher) Enrich(ctx context.Context, b *BleveIndexer, document Document) error {
	_, err := p.enrich(ctx, b, document)
	return err
}

func (b *BleveIndexer) goodReadsProvider() *GoodReadsEnricher {
	for _, provider := range b.documentEnrichers {
		if p, ok := provider.(*GoodReadsEnricher); ok {
			return p
		}
	}
	return nil
}

var dataSourceFields = []string{"DataSources", "DataSourceScoreValue"}

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

func goodReadsAuthor(document Document, matchedName string) (string, string) {
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

// storeGoodReadsAuthorURL requires documentsMu to be held.
func (b *BleveIndexer) storeGoodReadsAuthorURL(document Document, authorURL, matchedName string) error {
	if authorURL == "" {
		return nil
	}
	authorSlug, name := goodReadsAuthor(document, matchedName)
	if authorSlug == "" {
		return nil
	}
	b.authorsMu.Lock()
	defer b.authorsMu.Unlock()
	author, err := b.authorBySlugLocked(authorSlug)
	if err != nil {
		return err
	}
	if author.Slug == "" {
		author = Author{Slug: authorSlug, Name: name}
		for _, field := range []string{"AuthorsSlugs", "IllustratorsSlugs"} {
			q := bleve.NewTermQuery(authorSlug)
			q.SetField(field)
			result, err := b.documentsIdx.Search(bleve.NewSearchRequestOptions(q, 0, 0, false))
			if err != nil {
				return fmt.Errorf("counting documents for author %s: %w", authorSlug, err)
			}
			author.DocumentCount += result.Total
		}
	}
	if author.DataSourceLinks["goodreads"] != "" {
		return nil
	}
	if author.DataSourceLinks == nil {
		author.DataSourceLinks = make(map[string]string)
	}
	author.DataSourceLinks["goodreads"] = authorURL
	return indexAuthor(b.authorsIdx, authorSlug, author)
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

func matchingGoodReadsBook(document Document, results []biblioreads.SearchResult) (biblioreads.SearchResult, bool) {
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
func goodReadsQuery(document Document) string {
	q := strings.TrimSpace(document.Title)
	if len(document.Authors) > 0 {
		q = strings.TrimSpace(q + " " + document.Authors[0])
	}
	return q
}

func (b *BleveIndexer) documentsNeedingGoodReads(size int) ([]Document, uint64, error) {
	return b.documentsNeedingProvider(biblioreads.Source, size)
}

// GoodReadsBook returns the stored book URL and rating, or searches a document
// not looked up yet and persists its match. Returns an empty result if there's no match.
func (b *BleveIndexer) GoodReadsBook(ctx context.Context, document Document) (biblioreads.SearchResult, error) {
	data, err := DataSourceData[biblioreads.DocumentData](document, biblioreads.Source)
	if err != nil {
		return biblioreads.SearchResult{}, err
	}
	if data.URL != "" || data.Enriched || b.goodReadsProvider() == nil {
		return biblioreads.SearchResult{BookURL: data.URL, Rating: fmt.Sprintf("%.2f", document.DataSourceScoreValue), RatingCount: data.RatingCount}, nil
	}
	return b.enrichGoodReads(ctx, document)
}

// UpdateDataSourceLink saves a provider-normalized link and clears data cached for the previous match.
func (b *BleveIndexer) UpdateDataSourceLink(documentID, reviewLink string) error {
	b.documentsMu.Lock()
	defer b.documentsMu.Unlock()
	current, found, err := b.documentByIndexIDLocked(documentID)
	if err != nil {
		return fmt.Errorf("reading document %s for review-link correction: %w", documentID, err)
	}
	if !found {
		return ErrDocumentNotFound
	}
	data, err := DataSourceData[biblioreads.DocumentData](current, biblioreads.Source)
	if err != nil {
		return err
	}
	if data.URL == reviewLink {
		return nil
	}
	current.DataSourceScoreValue = 0
	if err := SetDataSourceData(&current, biblioreads.Source, biblioreads.DocumentData{URL: reviewLink, Enriched: true}); err != nil {
		return err
	}
	if err := IndexDocument(b.documentsIdx, current.ID, current); err != nil {
		return fmt.Errorf("saving review-link correction for document %s: %w", documentID, err)
	}
	return nil
}

// StoreGoodReadsDetails persists a nonempty rating, rating count, and complete review identifiers,
// provided the document still has the metadata and match read before the fetch.
func (b *BleveIndexer) StoreGoodReadsDetails(document Document, bookURL string, book biblioreads.BookResponse) error {
	if bookURL == "" {
		return fmt.Errorf("invalid GoodReads book URL for document %s", document.ID)
	}
	hasIDs := book.ResourceID != "" && book.LegacyBookID > 0
	if book.Rating == "" && book.RatingCount == "" && !hasIDs {
		return nil
	}
	b.documentsMu.Lock()
	defer b.documentsMu.Unlock()
	current, found, err := b.documentByIndexIDLocked(document.ID)
	if err != nil {
		return fmt.Errorf("error reading document %s for GoodReads details: %w", document.ID, err)
	}
	if !found {
		return ErrDocumentNotFound
	}
	data, err := DataSourceData[biblioreads.DocumentData](current, biblioreads.Source)
	if err != nil {
		return err
	}
	if goodReadsQuery(current) != goodReadsQuery(document) || data.URL != bookURL {
		return fmt.Errorf("GoodReads match changed for document %s", document.ID)
	}
	changed := false
	if book.RatingCount != "" && data.RatingCount != book.RatingCount {
		data.RatingCount = book.RatingCount
		changed = true
	}
	if book.Rating != "" {
		ratingValue := parseGoodReadsRating(book.Rating, document.ID)
		if current.DataSourceScoreValue != ratingValue {
			current.DataSourceScoreValue = ratingValue
			changed = true
		}
	}
	if hasIDs && (data.ResourceID != book.ResourceID || data.LegacyBookID != book.LegacyBookID) {
		data.ResourceID = book.ResourceID
		data.LegacyBookID = book.LegacyBookID
		changed = true
	}
	if !changed {
		return nil
	}
	if err := SetDataSourceData(&current, biblioreads.Source, data); err != nil {
		return err
	}
	if err := IndexDocument(b.documentsIdx, current.ID, current); err != nil {
		return fmt.Errorf("error storing GoodReads details for document %s: %w", document.ID, err)
	}
	return nil
}

// enrichGoodReads searches document on Goodreads, unless it already has a
// book URL, and stores matching URLs on the document and author respectively.
// Unavailability or persistence errors leave the document unenriched for retry.
func (b *BleveIndexer) enrichGoodReads(ctx context.Context, document Document) (biblioreads.SearchResult, error) {
	provider := b.goodReadsProvider()
	if provider == nil {
		return biblioreads.SearchResult{}, fmt.Errorf("Goodreads enrichment is not configured")
	}
	return provider.enrich(ctx, b, document)
}

func (p *GoodReadsEnricher) enrich(ctx context.Context, b *BleveIndexer, document Document) (biblioreads.SearchResult, error) {
	data, err := DataSourceData[biblioreads.DocumentData](document, biblioreads.Source)
	if err != nil {
		return biblioreads.SearchResult{}, err
	}
	select {
	case p.slots <- struct{}{}:
		defer func() { <-p.slots }()
	case <-ctx.Done():
		return biblioreads.SearchResult{}, ctx.Err()
	}
	q := goodReadsQuery(document)
	result := biblioreads.SearchResult{BookURL: data.URL}
	if result.BookURL == "" && document.Title != "" {
		ctx, cancel := context.WithTimeout(ctx, goodReadsRequestTimeout)
		defer cancel()
		queries := []string{q}
		if title := strings.TrimSpace(document.Title); title != q {
			queries = append(queries, title)
		}
		for _, query := range queries {
			res, err := p.source.SearchBooks(ctx, query)
			switch {
			case errors.Is(err, biblioreads.ErrUnavailable), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
				return biblioreads.SearchResult{}, fmt.Errorf("error searching %q on GoodReads: %w", query, err)
			case err != nil:
				log.Printf("Error searching %q on GoodReads: %s\n", query, err)
			default:
				if match, ok := matchingGoodReadsBook(document, res.Result); ok {
					result = match
				}
			}
			if result.BookURL != "" {
				break
			}
		}
	}

	// Re-read the document under the write lock, so concurrent writes of other
	// fields (e.g. TextRank enrichment) aren't overwritten with stale values.
	b.documentsMu.Lock()
	defer b.documentsMu.Unlock()
	current, found, err := b.documentByIndexIDLocked(document.ID)
	if err != nil {
		log.Printf("Error reading document %s for GoodReads enrichment: %s\n", document.ID, err)
		return result, nil
	}
	// Removed, or reindexed with a different title/author in the meantime
	// (which schedules its own lookup).
	if !found || goodReadsQuery(current) != q {
		return result, nil
	}
	data, err = DataSourceData[biblioreads.DocumentData](current, biblioreads.Source)
	if err != nil {
		return result, err
	}
	// Looked up concurrently (e.g. by the background enrichment and a detail
	// page request at the same time): keep the stored result.
	if data.Enriched {
		result = biblioreads.SearchResult{BookURL: data.URL, Rating: fmt.Sprintf("%.2f", current.DataSourceScoreValue), RatingCount: data.RatingCount}
		return result, nil
	}
	if data.URL != "" && data.URL != result.BookURL {
		result = biblioreads.SearchResult{BookURL: data.URL, Rating: fmt.Sprintf("%.2f", current.DataSourceScoreValue), RatingCount: data.RatingCount}
	}
	data.URL = result.BookURL
	if result.Rating == "" && current.DataSourceScoreValue > 0 {
		result.Rating = fmt.Sprintf("%.2f", current.DataSourceScoreValue)
	}
	current.DataSourceScoreValue = parseGoodReadsRating(result.Rating, current.ID)
	data.RatingCount = result.RatingCount
	data.Enriched = true
	if err := SetDataSourceData(&current, biblioreads.Source, data); err != nil {
		return result, err
	}
	if err := b.storeGoodReadsAuthorURL(current, result.AuthorURL, result.Author); err != nil {
		return result, fmt.Errorf("error storing GoodReads author URL for document %s: %w", current.ID, err)
	}
	if err := IndexDocument(b.documentsIdx, current.ID, current); err != nil {
		return result, fmt.Errorf("error indexing GoodReads-enriched document %s: %w", current.ID, err)
	}
	return result, nil
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

// documentByIndexIDLocked is documentByIndexID for callers already holding documentsMu.
func (b *BleveIndexer) documentByIndexIDLocked(id string) (Document, bool, error) {
	searchReq := bleve.NewSearchRequest(bleve.NewDocIDQuery([]string{id}))
	searchReq.Fields = []string{"*"}
	searchResult, err := b.documentsIdx.Search(searchReq)
	if err != nil || len(searchResult.Hits) == 0 {
		return Document{}, false, err
	}
	return hydrateDocument(searchResult.Hits[0]), true, nil
}

// preserveDataSourcesLocked copies currently stored provider data into
// documents, so writers that rebuilt them from an earlier read don't discard
// a lookup persisted in the meantime. Must be called with documentsMu held
// for writing, right before writing documents back.
func (b *BleveIndexer) preserveDataSourcesLocked(documents []Document) error {
	if len(documents) == 0 {
		return nil
	}
	ids := make([]string, len(documents))
	for i, document := range documents {
		ids[i] = document.ID
	}
	searchReq := bleve.NewSearchRequestOptions(bleve.NewDocIDQuery(ids), len(ids), 0, false)
	searchReq.Fields = dataSourceFields
	searchResult, err := b.documentsIdx.Search(searchReq)
	if err != nil {
		return fmt.Errorf("reading stored data source fields: %w", err)
	}
	stored := make(map[string]Document, len(searchResult.Hits))
	for _, hit := range searchResult.Hits {
		ratingValue, _ := hit.Fields["DataSourceScoreValue"].(float64)
		sources, err := decodeDataSources(hit.Fields, hit.ID)
		if err != nil {
			return err
		}
		stored[hit.ID] = Document{DataSources: sources, DataSourceScoreValue: ratingValue}
	}
	for i := range documents {
		if s, ok := stored[documents[i].ID]; ok {
			merged := maps.Clone(documents[i].DataSources)
			if merged == nil && len(s.DataSources) > 0 {
				merged = make(map[string]json.RawMessage)
			}
			maps.Copy(merged, s.DataSources)
			documents[i].DataSources = merged
			documents[i].DataSourceScoreValue = s.DataSourceScoreValue
		}
	}
	return nil
}
