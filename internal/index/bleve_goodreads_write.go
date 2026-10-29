package index

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"golang.org/x/text/unicode/norm"
)

const (
	// DefaultGoodReadsEnrichInterval is the minimum time between two GoodReads lookups.
	DefaultGoodReadsEnrichInterval = time.Second
	goodReadsRequestTimeout        = 2 * time.Minute
)

// GoodReadsSource searches books on Goodreads; implemented by *biblioreads.Client.
type GoodReadsSource interface {
	SearchBooks(ctx context.Context, query string) (biblioreads.SearchBooksResponse, error)
}

var goodReadsFields = []string{"GoodReadsBookURL", "GoodReadsAuthorURL", "GoodReadsResourceID", "GoodReadsLegacyBookID", "GoodReadsEnriched"}

// Strip named series annotations and omnibus ranges, not standalone volume numbers.
var goodReadsSeriesSuffix = regexp.MustCompile(`\s*\((?:[^()]+,\s*#[0-9]+(?:\.[0-9]+)?|#[0-9]+\s*[-–]\s*#?[0-9]+)\)\s*$`)

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

func goodReadsTitle(value string) string {
	return normalizeGoodReadsName(goodReadsSeriesSuffix.ReplaceAllString(value, ""))
}

func matchingGoodReadsBook(document Document, results []biblioreads.SearchResult) (biblioreads.SearchResult, bool) {
	title := goodReadsTitle(document.Title)
	if title == "" {
		return biblioreads.SearchResult{}, false
	}
	var best biblioreads.SearchResult
	bestScore := 0
	for _, result := range results {
		if result.BookURL == "" || goodReadsTitle(result.Title) != title {
			continue
		}
		author := strings.ReplaceAll(normalizeGoodReadsName(result.Author), " ", "")
		if author == "" {
			continue
		}
		for _, name := range document.Authors {
			if strings.ReplaceAll(normalizeGoodReadsName(name), " ", "") != author {
				continue
			}
			score := 1
			if normalizeGoodReadsName(result.Title) == normalizeGoodReadsName(document.Title) {
				score = 2
			}
			if score > bestScore {
				best, bestScore = result, score
			}
			break
		}
	}
	return best, bestScore > 0
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
// Document.GoodReadsEnriched), storing the book and author URLs of a matching
// search result. Lookups are throttled (see Config.GoodReadsEnrichInterval).
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
		for _, document := range chunk {
			if _, err := b.enrichGoodReads(context.Background(), document, true); err != nil {
				return err
			}
		}
	}

	log.Printf("GoodReads enrichment finished")
	return nil
}

// GoodReadsBookURL returns the Goodreads book path of document (see
// Document.GoodReadsBookURL). If the document hasn't been looked up yet, it
// searches it right away (without waiting for the background enrichment's
// throttle) and persists the result, so the background enrichment skips it.
// Returns an empty string if there's no match or GoodReads is disabled.
func (b *BleveIndexer) GoodReadsBookURL(ctx context.Context, document Document) (string, error) {
	if document.GoodReadsBookURL != "" || document.GoodReadsEnriched || b.goodReadsSource == nil {
		return document.GoodReadsBookURL, nil
	}
	return b.enrichGoodReads(ctx, document, false)
}

// StoreGoodReadsBookIDs persists identifiers fetched for bookURL, provided the
// document still has the same metadata and Goodreads match as when it was read.
func (b *BleveIndexer) StoreGoodReadsBookIDs(document Document, bookURL, resourceID string, legacyBookID int) error {
	if bookURL == "" || resourceID == "" || legacyBookID <= 0 {
		return fmt.Errorf("invalid GoodReads identifiers for document %s", document.ID)
	}
	b.documentsMu.Lock()
	defer b.documentsMu.Unlock()
	current, found, err := b.documentByIndexIDLocked(document.ID)
	if err != nil {
		return fmt.Errorf("error reading document %s for GoodReads identifiers: %w", document.ID, err)
	}
	if !found {
		return ErrDocumentNotFound
	}
	if goodReadsQuery(current) != goodReadsQuery(document) || current.GoodReadsBookURL != bookURL {
		return fmt.Errorf("GoodReads match changed for document %s", document.ID)
	}
	if current.GoodReadsResourceID == resourceID && current.GoodReadsLegacyBookID == legacyBookID {
		return nil
	}
	current.GoodReadsResourceID = resourceID
	current.GoodReadsLegacyBookID = legacyBookID
	if err := b.documentsIdx.Index(current.ID, current); err != nil {
		return fmt.Errorf("error storing GoodReads identifiers for document %s: %w", document.ID, err)
	}
	return nil
}

// enrichGoodReads searches document on Goodreads, unless it already has a
// book URL, and persists the first result's URLs, returning the book one.
// Only errors meaning Goodreads is unreachable are returned; in that case
// the document is left unenriched to be retried later.
func (b *BleveIndexer) enrichGoodReads(ctx context.Context, document Document, throttle bool) (string, error) {
	q := goodReadsQuery(document)
	bookURL, authorURL := document.GoodReadsBookURL, document.GoodReadsAuthorURL
	if bookURL == "" && document.Title != "" {
		ctx, cancel := context.WithTimeout(ctx, goodReadsRequestTimeout)
		defer cancel()
		queries := []string{q}
		if title := strings.TrimSpace(document.Title); title != q {
			queries = append(queries, title)
		}
		for _, query := range queries {
			if throttle {
				b.waitGoodReadsTurn()
			}
			res, err := b.goodReadsSource.SearchBooks(ctx, query)
			switch {
			case errors.Is(err, biblioreads.ErrUnavailable), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
				return "", fmt.Errorf("error searching %q on GoodReads: %w", query, err)
			case err != nil:
				log.Printf("Error searching %q on GoodReads: %s\n", query, err)
			default:
				if match, ok := matchingGoodReadsBook(document, res.Result); ok {
					bookURL, authorURL = match.BookURL, match.AuthorURL
				}
			}
			if bookURL != "" {
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
		return bookURL, nil
	}
	// Removed, or reindexed with a different title/author in the meantime
	// (which schedules its own lookup).
	if !found || goodReadsQuery(current) != q {
		return bookURL, nil
	}
	// Looked up concurrently (e.g. by the background enrichment and a detail
	// page request at the same time): keep the stored result.
	if current.GoodReadsEnriched {
		return current.GoodReadsBookURL, nil
	}
	if current.GoodReadsBookURL != "" {
		bookURL, authorURL = current.GoodReadsBookURL, current.GoodReadsAuthorURL
	}
	current.GoodReadsBookURL = bookURL
	current.GoodReadsAuthorURL = authorURL
	current.GoodReadsEnriched = true
	if err := b.documentsIdx.Index(current.ID, current); err != nil {
		log.Printf("Error indexing GoodReads-enriched document %s: %s\n", current.ID, err)
	}
	return bookURL, nil
}

// waitGoodReadsTurn blocks until at least goodReadsInterval has passed since
// the previous GoodReads lookup, shared by EnrichGoodReads and the per-file
// worker.
func (b *BleveIndexer) waitGoodReadsTurn() {
	b.goodReadsThrottleMu.Lock()
	defer b.goodReadsThrottleMu.Unlock()
	if wait := time.Until(b.goodReadsLastRequest.Add(b.goodReadsInterval)); wait > 0 {
		time.Sleep(wait)
	}
	b.goodReadsLastRequest = time.Now()
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
		if _, err := b.enrichGoodReads(context.Background(), document, true); err != nil {
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
		authorURL, _ := hit.Fields["GoodReadsAuthorURL"].(string)
		resourceID, _ := hit.Fields["GoodReadsResourceID"].(string)
		legacyBookID, _ := hit.Fields["GoodReadsLegacyBookID"].(float64)
		enriched, _ := hit.Fields["GoodReadsEnriched"].(bool)
		stored[hit.ID] = Document{GoodReadsBookURL: bookURL, GoodReadsAuthorURL: authorURL, GoodReadsResourceID: resourceID, GoodReadsLegacyBookID: int(legacyBookID), GoodReadsEnriched: enriched}
	}
	for i := range documents {
		if s, ok := stored[documents[i].ID]; ok {
			documents[i].GoodReadsBookURL = s.GoodReadsBookURL
			documents[i].GoodReadsAuthorURL = s.GoodReadsAuthorURL
			documents[i].GoodReadsResourceID = s.GoodReadsResourceID
			documents[i].GoodReadsLegacyBookID = s.GoodReadsLegacyBookID
			documents[i].GoodReadsEnriched = s.GoodReadsEnriched
		}
	}
}
