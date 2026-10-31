package index

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"golang.org/x/text/unicode/norm"
)

const (
	goodReadsEnrichWorkers  = 4
	goodReadsRequestTimeout = 2 * time.Minute
)

// GoodReadsSource searches books on Goodreads; implemented by *biblioreads.Client.
type GoodReadsSource interface {
	SearchBooks(ctx context.Context, query string) (biblioreads.SearchBooksResponse, error)
}

var goodReadsFields = []string{"GoodReadsBookURL", "GoodReadsRating", "GoodReadsRatingValue", "GoodReadsResourceID", "GoodReadsLegacyBookID", "GoodReadsEnriched"}

// Strip named series annotations and omnibus ranges, not standalone volume numbers.
var goodReadsSeriesSuffix = regexp.MustCompile(`\s*\((?:[^()]+,\s*#[0-9]+(?:\.[0-9]+)?|#[0-9]+\s*[-–]\s*#?[0-9]+)\)\s*$`)

var goodReadsSubtitleSeparator = regexp.MustCompile(`:\s*|\.\s+|\s+[-–—]\s+`)

var goodReadsParentheticalTitle = regexp.MustCompile(`\s+\([\p{L}][^()]*\)\s*$`)

var goodReadsBookPath = regexp.MustCompile(`^/book/show/[1-9][0-9]*(?:[.-][A-Za-z0-9._~-]+)?/?$`)

var ErrInvalidGoodReadsBookURL = errors.New("invalid Goodreads book URL")

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
	return "", ""
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
	if author.GoodReadsAuthorURL != "" {
		return nil
	}
	author.GoodReadsAuthorURL = authorURL
	return b.authorsIdx.Index(authorSlug, author)
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

func goodReadsTitleMatchScore(candidate, title, fullTitle, mainTitle string) int {
	candidateTitle := goodReadsTitle(candidate)
	if candidateTitle == "" {
		return 0
	}
	if candidateTitle == title {
		if normalizeGoodReadsName(candidate) == fullTitle {
			return 3
		}
		return 2
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
	if score == bestScore && candidate.ID > 0 && best.ID > 0 && candidate.ID != best.ID {
		return candidate.ID < best.ID
	}
	hasRating, bestHasRating := hasGoodReadsRating(candidate.Rating), hasGoodReadsRating(best.Rating)
	if hasRating != bestHasRating {
		return hasRating
	}
	return score > bestScore
}

func matchingGoodReadsBook(document Document, results []biblioreads.SearchResult) (biblioreads.SearchResult, bool) {
	title := goodReadsTitle(document.Title)
	if title == "" {
		return biblioreads.SearchResult{}, false
	}
	fullTitle := normalizeGoodReadsName(document.Title)
	mainTitle := goodReadsMainTitle(document.Title)
	authors := make(map[string]struct{}, len(document.Authors))
	for _, name := range document.Authors {
		authors[strings.ReplaceAll(normalizeGoodReadsName(name), " ", "")] = struct{}{}
	}
	var best biblioreads.SearchResult
	bestScore := 0
	for _, result := range results {
		if result.BookURL == "" {
			continue
		}
		score := goodReadsTitleMatchScore(result.Title, title, fullTitle, mainTitle)
		if score == 0 {
			continue
		}
		author := strings.ReplaceAll(normalizeGoodReadsName(result.Author), " ", "")
		if author == "" {
			continue
		}
		if _, matches := authors[author]; matches && preferGoodReadsBook(result, best, score, bestScore) {
			best, bestScore = result, score
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

func documentsNeedingGoodReadsQuery() query.Query {
	notEnriched := bleve.NewBoolFieldQuery(false)
	notEnriched.SetField("GoodReadsEnriched")
	return notEnriched
}

func (b *BleveIndexer) documentsNeedingGoodReads(size int) ([]Document, uint64, error) {
	searchReq := bleve.NewSearchRequestOptions(documentsNeedingGoodReadsQuery(), size, 0, false)
	searchReq.Fields = []string{"*"}
	b.documentsMu.RLock()
	searchResult, err := b.documentsIdx.Search(searchReq)
	b.documentsMu.RUnlock()
	if err != nil {
		return nil, 0, err
	}
	documents := make([]Document, 0, len(searchResult.Hits))
	for _, hit := range searchResult.Hits {
		documents = append(documents, hydrateDocument(hit))
	}
	return documents, searchResult.Total, nil
}

// EnrichGoodReads looks up on Goodreads every document not processed yet (see
// Document.GoodReadsEnriched), storing the book URL on the document and the
// author URL on the matching author. The source handles per-instance throttling.
// It stops at the first error that means Goodreads can't be reached right
// now, leaving the remaining documents for the next run.
func (b *BleveIndexer) EnrichGoodReads(batchSize int) error {
	if b.goodReadsSource == nil {
		return nil
	}

	_, total, err := b.documentsNeedingGoodReads(0)
	if err != nil || total == 0 {
		return err
	}
	log.Printf("Enriching %d documents with GoodReads data", total)

	// Bounds the loop if a document's write keeps failing, which would leave
	// it in documentsNeedingGoodReads' results forever.
	maxIterations := int(total) + 10
	for iterations := 0; ; iterations++ {
		if iterations >= maxIterations {
			return fmt.Errorf("GoodReads enrichment did not finish after %d iterations, some documents may be stuck unenriched", iterations)
		}
		// Enriched documents drop out of the query, so it always starts from 0.
		chunk, _, err := b.documentsNeedingGoodReads(batchSize)
		if err != nil {
			return err
		}
		if len(chunk) == 0 {
			break
		}
		if err := b.enrichGoodReadsBatch(chunk); err != nil {
			return err
		}
	}

	log.Printf("GoodReads enrichment finished")
	return nil
}

func (b *BleveIndexer) enrichGoodReadsBatch(documents []Document) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobs := make(chan Document, len(documents))
	for _, document := range documents {
		jobs <- document
	}
	close(jobs)
	var wg sync.WaitGroup
	errs := make(chan error, 1)
	for range min(goodReadsEnrichWorkers, len(documents)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for document := range jobs {
				if _, err := b.enrichGoodReads(ctx, document); err != nil {
					select {
					case errs <- err:
						cancel()
					default:
					}
					return
				}
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
}

// GoodReadsBook returns the stored book URL and rating, or searches a document
// not looked up yet and persists its match. Returns an empty result if there's no match.
func (b *BleveIndexer) GoodReadsBook(ctx context.Context, document Document) (biblioreads.SearchResult, error) {
	if document.GoodReadsBookURL != "" || document.GoodReadsEnriched || b.goodReadsSource == nil {
		return biblioreads.SearchResult{BookURL: document.GoodReadsBookURL, Rating: document.GoodReadsRating}, nil
	}
	return b.enrichGoodReads(ctx, document)
}

// UpdateGoodReadsBookURL saves an administrator's correction without retaining
// the previous book's rating or review identifiers.
func (b *BleveIndexer) UpdateGoodReadsBookURL(documentID, rawURL string) error {
	bookURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || len(rawURL) > 2048 || bookURL.User != nil ||
		(bookURL.Scheme != "https" && bookURL.Scheme != "http") ||
		(bookURL.Host != "www.goodreads.com" && bookURL.Host != "goodreads.com") ||
		!goodReadsBookPath.MatchString(bookURL.Path) {
		return ErrInvalidGoodReadsBookURL
	}
	path := strings.TrimSuffix(bookURL.Path, "/")

	b.documentsMu.Lock()
	defer b.documentsMu.Unlock()
	current, found, err := b.documentByIndexIDLocked(documentID)
	if err != nil {
		return fmt.Errorf("reading document %s for Goodreads correction: %w", documentID, err)
	}
	if !found {
		return ErrDocumentNotFound
	}
	if current.GoodReadsBookURL == path {
		return nil
	}
	current.GoodReadsBookURL = path
	current.GoodReadsRating = ""
	current.GoodReadsRatingValue = 0
	current.GoodReadsResourceID = ""
	current.GoodReadsLegacyBookID = 0
	current.GoodReadsEnriched = true
	if err := b.documentsIdx.Index(current.ID, current); err != nil {
		return fmt.Errorf("saving Goodreads correction for document %s: %w", documentID, err)
	}
	return nil
}

// StoreGoodReadsDetails persists a nonempty rating and complete review identifiers,
// provided the document still has the metadata and match read before the fetch.
func (b *BleveIndexer) StoreGoodReadsDetails(document Document, bookURL string, book biblioreads.BookResponse) error {
	if bookURL == "" {
		return fmt.Errorf("invalid GoodReads book URL for document %s", document.ID)
	}
	hasIDs := book.ResourceID != "" && book.LegacyBookID > 0
	if book.Rating == "" && !hasIDs {
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
	if goodReadsQuery(current) != goodReadsQuery(document) || current.GoodReadsBookURL != bookURL {
		return fmt.Errorf("GoodReads match changed for document %s", document.ID)
	}
	changed := false
	if book.Rating != "" {
		ratingValue := parseGoodReadsRating(book.Rating, document.ID)
		if current.GoodReadsRating != book.Rating || current.GoodReadsRatingValue != ratingValue {
			current.GoodReadsRating = book.Rating
			current.GoodReadsRatingValue = ratingValue
			changed = true
		}
	}
	if hasIDs && (current.GoodReadsResourceID != book.ResourceID || current.GoodReadsLegacyBookID != book.LegacyBookID) {
		current.GoodReadsResourceID = book.ResourceID
		current.GoodReadsLegacyBookID = book.LegacyBookID
		changed = true
	}
	if !changed {
		return nil
	}
	if err := b.documentsIdx.Index(current.ID, current); err != nil {
		return fmt.Errorf("error storing GoodReads details for document %s: %w", document.ID, err)
	}
	return nil
}

// enrichGoodReads searches document on Goodreads, unless it already has a
// book URL, and stores matching URLs on the document and author respectively.
// Unavailability or persistence errors leave the document unenriched for retry.
func (b *BleveIndexer) enrichGoodReads(ctx context.Context, document Document) (biblioreads.SearchResult, error) {
	select {
	case b.goodReadsSlots <- struct{}{}:
		defer func() { <-b.goodReadsSlots }()
	case <-ctx.Done():
		return biblioreads.SearchResult{}, ctx.Err()
	}
	q := goodReadsQuery(document)
	result := biblioreads.SearchResult{BookURL: document.GoodReadsBookURL}
	if result.BookURL == "" && document.Title != "" {
		ctx, cancel := context.WithTimeout(ctx, goodReadsRequestTimeout)
		defer cancel()
		queries := []string{q}
		if title := strings.TrimSpace(document.Title); title != q {
			queries = append(queries, title)
		}
		for _, query := range queries {
			res, err := b.goodReadsSource.SearchBooks(ctx, query)
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
	// Looked up concurrently (e.g. by the background enrichment and a detail
	// page request at the same time): keep the stored result.
	if current.GoodReadsEnriched {
		result = biblioreads.SearchResult{BookURL: current.GoodReadsBookURL, Rating: current.GoodReadsRating}
		return result, nil
	}
	if current.GoodReadsBookURL != "" && current.GoodReadsBookURL != result.BookURL {
		result = biblioreads.SearchResult{BookURL: current.GoodReadsBookURL, Rating: current.GoodReadsRating}
	}
	current.GoodReadsBookURL = result.BookURL
	if result.Rating == "" {
		result.Rating = current.GoodReadsRating
	}
	current.GoodReadsRating = result.Rating
	current.GoodReadsRatingValue = parseGoodReadsRating(result.Rating, current.ID)
	current.GoodReadsEnriched = true
	if err := b.storeGoodReadsAuthorURL(current, result.AuthorURL, result.Author); err != nil {
		return result, fmt.Errorf("error storing GoodReads author URL for document %s: %w", current.ID, err)
	}
	if err := b.documentsIdx.Index(current.ID, current); err != nil {
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

// runGoodReadsEnrichWorker looks up documents queued by indexFile for the
// indexer's lifetime.
func (b *BleveIndexer) runGoodReadsEnrichWorker() {
	for id := range b.goodReadsEnrichJobs {
		document, err := b.documentByIndexID(id)
		if err != nil {
			log.Printf("Error reading document %s for GoodReads enrichment: %s\n", id, err)
			continue
		}
		if document.ID == "" || document.GoodReadsEnriched {
			continue
		}
		if _, err := b.enrichGoodReads(context.Background(), document); err != nil {
			log.Println(err)
		}
	}
}

// scheduleGoodReadsEnrichment queues the document with the given ID for
// lookup from a throwaway goroutine, so indexFile never blocks on it.
func (b *BleveIndexer) scheduleGoodReadsEnrichment(id string) {
	if b.goodReadsSource == nil {
		return
	}
	go func() {
		b.goodReadsEnrichJobs <- id
	}()
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

// preserveGoodReadsLocked copies the currently stored GoodReads fields into
// documents, so writers that rebuilt them from an earlier read don't discard
// a lookup persisted in the meantime. Must be called with documentsMu held
// for writing, right before writing documents back.
func (b *BleveIndexer) preserveGoodReadsLocked(documents []Document) {
	if len(documents) == 0 {
		return
	}
	ids := make([]string, len(documents))
	for i, document := range documents {
		ids[i] = document.ID
	}
	searchReq := bleve.NewSearchRequestOptions(bleve.NewDocIDQuery(ids), len(ids), 0, false)
	searchReq.Fields = goodReadsFields
	searchResult, err := b.documentsIdx.Search(searchReq)
	if err != nil {
		log.Printf("Error reading stored GoodReads fields: %s\n", err)
		return
	}
	stored := make(map[string]Document, len(searchResult.Hits))
	for _, hit := range searchResult.Hits {
		bookURL, _ := hit.Fields["GoodReadsBookURL"].(string)
		rating, _ := hit.Fields["GoodReadsRating"].(string)
		ratingValue, _ := hit.Fields["GoodReadsRatingValue"].(float64)
		if ratingValue == 0 {
			ratingValue = parseGoodReadsRating(rating, hit.ID)
		}
		resourceID, _ := hit.Fields["GoodReadsResourceID"].(string)
		legacyBookID, _ := hit.Fields["GoodReadsLegacyBookID"].(float64)
		enriched, _ := hit.Fields["GoodReadsEnriched"].(bool)
		stored[hit.ID] = Document{GoodReadsBookURL: bookURL, GoodReadsRating: rating, GoodReadsRatingValue: ratingValue, GoodReadsResourceID: resourceID, GoodReadsLegacyBookID: int(legacyBookID), GoodReadsEnriched: enriched}
	}
	for i := range documents {
		if s, ok := stored[documents[i].ID]; ok {
			documents[i].GoodReadsBookURL = s.GoodReadsBookURL
			documents[i].GoodReadsRating = s.GoodReadsRating
			documents[i].GoodReadsRatingValue = s.GoodReadsRatingValue
			documents[i].GoodReadsResourceID = s.GoodReadsResourceID
			documents[i].GoodReadsLegacyBookID = s.GoodReadsLegacyBookID
			documents[i].GoodReadsEnriched = s.GoodReadsEnriched
		}
	}
}
