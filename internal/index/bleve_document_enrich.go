package index

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/blevesearch/bleve/v2"
)

// documentEnrichWorkers sets lookup parallelism for each pool. Enrichers must enforce
// their own source request limits because batch and upload pools run concurrently.
const documentEnrichWorkers = 4

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

func (b *BleveIndexer) documentsNeedingProvider(source string, size int) ([]Document, uint64, error) {
	enriched := bleve.NewTermQuery(source)
	enriched.SetField("EnrichedDataSources")
	q := bleve.NewBooleanQuery()
	q.AddMust(bleve.NewMatchAllQuery())
	q.AddMustNot(enriched)
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
	if batchSize <= 0 {
		return fmt.Errorf("document enrichment batch size must be positive")
	}
	var failures []error
	for i, provider := range b.documentEnrichers {
		if err := b.enrichProvider(provider, b.documentEnrichTracker(i), batchSize); err != nil {
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

func (b *BleveIndexer) enrichProvider(provider DocumentEnricher, tracker *progressTracker, batchSize int) error {
	_, total, err := b.documentsNeedingProvider(provider.Source(), 0)
	if err != nil || total == 0 {
		return err
	}
	tracker.begin(int(total))
	defer tracker.end()
	log.Printf("Enriching %d documents with %s data", total, provider.Source())
	for iteration := 0; iteration < int(total)+10; iteration++ {
		documents, _, err := b.documentsNeedingProvider(provider.Source(), batchSize)
		if err != nil {
			return err
		}
		if len(documents) == 0 {
			return nil
		}
		if err := b.enrichProviderBatch(provider, tracker, documents); err != nil {
			return err
		}
	}
	return fmt.Errorf("document enrichment did not finish; some documents remain pending")
}

func (b *BleveIndexer) enrichProviderBatch(provider DocumentEnricher, tracker *progressTracker, documents []Document) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobs := make(chan Document, len(documents))
	for _, document := range documents {
		jobs <- document
	}
	close(jobs)
	var workers sync.WaitGroup
	failures := make(chan error, 1)
	for range min(documentEnrichWorkers, len(documents)) {
		workers.Go(func() {
			for document := range jobs {
				needed, err := provider.NeedsEnrichment(document)
				if err == nil && needed {
					err = provider.Enrich(ctx, b, document)
				}
				if err != nil {
					select {
					case failures <- err:
						cancel()
					default:
					}
					return
				}
				tracker.record()
			}
		})
	}
	workers.Wait()
	select {
	case err := <-failures:
		return err
	default:
		return nil
	}
}

func (b *BleveIndexer) runDocumentEnrichWorker() {
	for id := range b.documentEnrichJobs {
		for _, provider := range b.documentEnrichers {
			document, err := b.documentByIndexID(id)
			if err != nil {
				log.Printf("Error reading document %s for %s enrichment: %s", id, provider.Source(), err)
				continue
			}
			if document.ID == "" {
				break
			}
			needed, err := provider.NeedsEnrichment(document)
			if err == nil && needed {
				err = provider.Enrich(context.Background(), b, document)
			}
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
