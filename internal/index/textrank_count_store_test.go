package index

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
)

func TestTextRankCountStoreFiltersAndCleansUp(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	store, err := newTextRankCountStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.close)
	store.limit = 256
	for i := range 25 {
		entries := map[textRankCountKey]struct{}{
			{"phrase", "common phrase"}:                 {},
			{"word", "common"}:                          {},
			{"subject", "physics"}:                      {},
			{"similarity", "too common"}:                {},
			{"similarity", fmt.Sprintf("unique %d", i)}: {},
		}
		if i < 20 {
			entries[textRankCountKey{"phrase", "boundary phrase"}] = struct{}{}
			entries[textRankCountKey{"similarity", "boundary pair"}] = struct{}{}
		}
		if i < 2 {
			entries[textRankCountKey{"similarity", "atomic bomb"}] = struct{}{}
		}
		if err := store.add(entries); err != nil {
			t.Fatal(err)
		}
		if i%3 == 0 {
			if err := store.flush(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := store.flush(); err != nil {
		t.Fatal(err)
	}
	counts, err := store.load(20, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(counts.phrases) != 1 || counts.phrases["common phrase"] != 25 ||
		len(counts.words) != 1 || counts.words["common"] != 25 ||
		len(counts.similarityPhrases) != 2 || counts.similarityPhrases["atomic bomb"] != 2 ||
		counts.similarityPhrases["boundary pair"] != 20 || counts.subjects["physics"] != 25 {
		t.Fatalf("incorrect filtered counts: %+v", counts)
	}
	store.close()
	if _, err := os.Stat(store.dir); !os.IsNotExist(err) {
		t.Fatalf("temporary counts were not removed: %v", err)
	}
}

func TestTextRankCountStorePreservesEntriesAcrossMerges(t *testing.T) {
	store, err := newTextRankCountStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.close)
	store.limit = 128
	entries := []string{"", "a\nb", "a\tb", "\"quoted\"", "ámbito físico", strings.Repeat("long", 20000)}
	for range 3 {
		for _, entry := range entries {
			if err := store.add(map[textRankCountKey]struct{}{{"subject", entry}: {}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	counts, err := store.load(20, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(counts.subjects) != len(entries) {
		t.Fatalf("got %d entries, want %d", len(counts.subjects), len(entries))
	}
	for _, entry := range entries {
		if counts.subjects[entry] != 3 {
			t.Fatalf("entry of length %d has count %d, want 3", len(entry), counts.subjects[entry])
		}
	}
}

func TestTextRankCountStoreRejectsCorruptRun(t *testing.T) {
	store, err := newTextRankCountStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.close)
	if err := store.add(map[textRankCountKey]struct{}{{"subject", "physics"}: {}}); err != nil {
		t.Fatal(err)
	}
	if err := store.flush(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.runs[0], []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.load(20, 20); err == nil {
		t.Fatal("corrupt counting run must return an explicit error")
	}
}

func TestTextRankCountingStorageFailureDoesNotPublish(t *testing.T) {
	idx, documents := newTestTextRankIndexer(t, Config{})
	t.Cleanup(func() { _ = idx.Close() })
	doc := evidenceDocument("reference", "atomic bomb")
	if err := IndexDocument(documents, doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	if err := idx.maintainTextRankEntries(10); err == nil {
		t.Fatal("unavailable counting storage must return an error")
	}
	for _, key := range [][]byte{internalDocCountAtLastPrune, internalSimilarityPhraseStatistics} {
		value, err := documents.GetInternal(key)
		if err != nil || len(value) != 0 {
			t.Fatalf("failed maintenance published %q: %q, %v", key, value, err)
		}
	}
}

func BenchmarkTextRankCountingMemory(b *testing.B) {
	for _, documents := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(documents), func(b *testing.B) {
			documentsIndex, err := bleve.NewMemOnly(CreateDocumentsMapping())
			if err != nil {
				b.Fatal(err)
			}
			defer documentsIndex.Close()
			idx := NewBleve(documentsIndex, nil, nil, "", nil, Config{})
			var peak uint64
			for b.Loop() {
				counts, err := idx.countTextRankEntries(uint64(documents), 10, func(from int) (*bleve.SearchResult, error) {
					runtime.GC()
					var memory runtime.MemStats
					runtime.ReadMemStats(&memory)
					peak = max(peak, memory.HeapAlloc)
					result := &bleve.SearchResult{}
					for i := from; i < min(from+10, documents); i++ {
						phrases := make([]interface{}, 100)
						for j := range phrases {
							phrases[j] = fmt.Sprintf("unique%d phrase%d", i, j)
						}
						result.Hits = append(result.Hits, &search.DocumentMatch{
							Fields: map[string]interface{}{"TextRankPhrases": phrases},
						})
					}
					return result, nil
				})
				if err != nil {
					b.Fatal(err)
				}
				if len(counts.phrases) != 0 || len(counts.similarityPhrases) != 0 {
					b.Fatal("unique phrases must not be retained")
				}
			}
			b.ReportMetric(float64(peak)/1024/1024, "live-heap-MiB")
		})
	}
}
