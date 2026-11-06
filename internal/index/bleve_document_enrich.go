package index

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blevesearch/bleve/v2"
)

// documentEnrichWorkers sets lookup parallelism for each pool. Enrichers must enforce
// their own source request limits because batch and upload pools run concurrently.
const documentEnrichWorkers = 4

const maxNotFoundAttempts = 3

// ErrEnrichmentNotFound marks an ambiguous provider 404, not a successful no-match lookup.
var ErrEnrichmentNotFound = errors.New("provider lookup returned not found")

type documentLookupState struct {
	mu               sync.Mutex
	notFoundAttempts atomic.Uint32
}

func (p *progressTracker) unresolvedDocuments() map[string]struct{} {
	unresolved := make(map[string]struct{})
	p.lookups.Range(func(key, value any) bool {
		state := value.(*documentLookupState)
		if state.notFoundAttempts.Load() >= maxNotFoundAttempts {
			unresolved[key.(string)] = struct{}{}
		}
		return true
	})
	return unresolved
}

// DocumentEnricher owns a provider's lookup policy and persisted payload.
type DocumentEnricher interface {
	// Source is the persisted storage key; Name is shown to users.
	Source() string
	Name() string
	NeedsEnrichment(Document) (bool, error)
	// Enrich persists an enriched=true payload for completed lookups, including no matches.
	// Transient failures must leave the document pending. Enrich may be called concurrently
	// by multiple pools, so implementations must limit their own requests to the source.
	Enrich(context.Context, *BleveIndexer, Document) error
}

func (b *BleveIndexer) documentsNeedingProvider(source string, size int, excluded map[string]struct{}) ([]Document, uint64, error) {
	enriched := bleve.NewTermQuery(source)
	enriched.SetField("EnrichedDataSources")
	q := bleve.NewBooleanQuery()
	q.AddMust(bleve.NewMatchAllQuery())
	q.AddMustNot(enriched)
	if len(excluded) > 0 {
		ids := make([]string, 0, len(excluded))
		for id := range excluded {
			ids = append(ids, id)
		}
		q.AddMustNot(bleve.NewDocIDQuery(ids))
	}
	req := bleve.NewSearchRequestOptions(q, size, 0, false)
	req.Fields = []string{"*"}
	b.documentsMu.RLock()
	result, err := b.documentsIdx.Search(req)
	b.documentsMu.RUnlock()
	if err != nil {
		return nil, 0, err
	}
	documents := make([]Document, 0, len(result.Hits))
	for _, hit := range result.Hits {
		documents = append(documents, hydrateDocument(hit))
	}
	return documents, result.Total, nil
}

// EnrichDocuments resumes pending lookups for every configured provider.
// A provider failure does not prevent independent providers from running.
func (b *BleveIndexer) EnrichDocuments(batchSize int) error {
	return b.enrichDocuments(context.Background(), batchSize)
}

// StartDocumentEnrichment retries pending lookups until the indexer is closed.
func (b *BleveIndexer) StartDocumentEnrichment(batchSize int) error {
	if batchSize <= 0 {
		return fmt.Errorf("document enrichment batch size must be positive")
	}
	if len(b.documentEnrichers) == 0 {
		return nil
	}
	b.documentEnrichStart.Do(func() {
		b.documentEnrichWG.Go(func() {
			b.retryDocumentEnrichment(b.documentEnrichContext, batchSize, time.Minute, 15*time.Minute)
		})
	})
	return nil
}

func (b *BleveIndexer) retryDocumentEnrichment(ctx context.Context, batchSize int, interval, maxDelay time.Duration) {
	delay := interval
	for ctx.Err() == nil {
		err := b.enrichDocuments(ctx, batchSize)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("Error enriching documents with provider data; retrying pending lookups in %s: %s", delay, err)
		} else {
			delay = interval
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if err != nil {
			delay = min(2*delay, maxDelay)
		}
	}
}

func (b *BleveIndexer) enrichDocuments(ctx context.Context, batchSize int) error {
	if batchSize <= 0 {
		return fmt.Errorf("document enrichment batch size must be positive")
	}
	var failures []error
	for i, provider := range b.documentEnrichers {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := b.enrichProvider(ctx, provider, b.documentEnrichTracker(i), batchSize); err != nil {
			failures = append(failures, fmt.Errorf("enriching documents with %s: %w", provider.Source(), err))
		}
	}
	return errors.Join(failures...)
}

func (b *BleveIndexer) documentEnrichTracker(i int) *progressTracker {
	if i < len(b.documentEnrichProgress) {
		return &b.documentEnrichProgress[i]
	}
	return &progressTracker{}
}

func (b *BleveIndexer) enrichProvider(ctx context.Context, provider DocumentEnricher, tracker *progressTracker, batchSize int) (resultErr error) {
	defer func() {
		tracker.failed.Store(resultErr != nil)
		tracker.end()
	}()
	failedDocuments := tracker.unresolvedDocuments()
	_, total, err := b.documentsNeedingProvider(provider.Source(), 0, failedDocuments)
	if err != nil || total == 0 {
		return err
	}
	tracker.begin(int(total))
	log.Printf("Enriching %d documents with %s data", total, provider.Source())
	var failures []error
	for iteration := 0; iteration < int(total)+10; iteration++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		documents, _, err := b.documentsNeedingProvider(provider.Source(), batchSize, failedDocuments)
		if err != nil {
			failures = append(failures, err)
			break
		}
		if len(documents) == 0 {
			return errors.Join(failures...)
		}
		for id, err := range b.enrichProviderBatch(ctx, provider, tracker, documents) {
			failedDocuments[id] = struct{}{}
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	return fmt.Errorf("document enrichment did not finish; some documents remain pending")
}

func (b *BleveIndexer) enrichProviderBatch(ctx context.Context, provider DocumentEnricher, tracker *progressTracker, documents []Document) map[string]error {
	jobs := make(chan Document, len(documents))
	for _, document := range documents {
		jobs <- document
	}
	close(jobs)
	var workers sync.WaitGroup
	var failureMu sync.Mutex
	failures := make(map[string]error)
	for range min(documentEnrichWorkers, len(documents)) {
		workers.Go(func() {
			for document := range jobs {
				if ctx.Err() != nil {
					return
				}
				err := b.enrichDocument(ctx, provider, tracker, document)
				if err != nil {
					failureMu.Lock()
					failures[document.ID] = fmt.Errorf("enriching document %s: %w", document.ID, err)
					failureMu.Unlock()
				}
				tracker.record()
			}
		})
	}
	workers.Wait()
	return failures
}

func (b *BleveIndexer) enrichDocument(ctx context.Context, provider DocumentEnricher, tracker *progressTracker, document Document) error {
	value, _ := tracker.lookups.LoadOrStore(document.ID, &documentLookupState{})
	state := value.(*documentLookupState)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.notFoundAttempts.Load() >= maxNotFoundAttempts {
		return fmt.Errorf("automatic retry limit reached after %d not-found responses", maxNotFoundAttempts)
	}
	needed, err := provider.NeedsEnrichment(document)
	if err == nil && needed {
		err = provider.Enrich(ctx, b, document)
	}
	if errors.Is(err, ErrEnrichmentNotFound) {
		if state.notFoundAttempts.Add(1) == maxNotFoundAttempts {
			b.providerProgressVersion.Add(1)
			log.Printf("Leaving document %s unresolved for %s after %d not-found responses: %s",
				document.ID, provider.Source(), maxNotFoundAttempts, err)
		}
	} else if err == nil {
		if state.notFoundAttempts.Swap(0) > 0 {
			b.providerProgressVersion.Add(1)
		}
	}
	return err
}

func (b *BleveIndexer) runDocumentEnrichWorker() {
	for id := range b.documentEnrichJobs {
		for i, provider := range b.documentEnrichers {
			document, err := b.documentByIndexID(id)
			if err != nil {
				log.Printf("Error reading document %s for %s enrichment: %s", id, provider.Source(), err)
				continue
			}
			if document.ID == "" {
				break
			}
			err = b.enrichDocument(context.Background(), provider, b.documentEnrichTracker(i), document)
			if err != nil {
				log.Printf("Error enriching document %s with %s: %s", id, provider.Source(), err)
			}
		}
	}
}

func (b *BleveIndexer) scheduleDocumentEnrichment(id string) {
	if len(b.documentEnrichers) > 0 {
		go func() { b.documentEnrichJobs <- id }()
	}
}

// StoreDocumentData replaces only one provider's payload under the document write lock.
func (b *BleveIndexer) StoreDocumentData(documentID, source string, data any) error {
	b.documentsMu.Lock()
	defer b.documentsMu.Unlock()
	document, found, err := b.documentByIndexIDLocked(documentID)
	if err != nil {
		return err
	}
	if !found {
		return ErrDocumentNotFound
	}
	if err := SetDataSourceData(&document, source, data); err != nil {
		return err
	}
	if err := IndexDocument(b.documentsIdx, documentID, document); err != nil {
		return err
	}
	b.providerProgressVersion.Add(1)
	return nil
}
