package index

import (
	"bufio"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
)

type textRankCountKey struct {
	kind, entry string
}

type textRankCountRecord struct {
	Kind  string
	Entry string
	Count int
}

func compareTextRankCounts(a, b textRankCountRecord) int {
	if order := cmp.Compare(a.Kind, b.Kind); order != 0 {
		return order
	}
	return cmp.Compare(a.Entry, b.Entry)
}

// Sorted runs keep counting memory bounded; each merge opens only three files.
type textRankCountStore struct {
	dir     string
	pending map[textRankCountKey]int
	bytes   int
	limit   int
	runs    []string
	next    int
}

func newTextRankCountStore() (*textRankCountStore, error) {
	dir, err := os.MkdirTemp("", "coreander-textrank-")
	if err != nil {
		return nil, fmt.Errorf("create TextRank counting directory: %w", err)
	}
	return &textRankCountStore{
		dir: dir, pending: make(map[textRankCountKey]int), limit: 1024 * 1024,
	}, nil
}

func (s *textRankCountStore) add(entries map[textRankCountKey]struct{}) error {
	for key := range entries {
		if s.pending[key] == 0 {
			s.bytes += len(key.kind) + len(key.entry) + 64
		}
		s.pending[key]++
		if s.bytes >= s.limit {
			if err := s.flush(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *textRankCountStore) writeRun(write func(*json.Encoder) error) (path string, err error) {
	path = filepath.Join(s.dir, fmt.Sprintf("run-%d", s.next))
	s.next++
	file, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create TextRank counting run: %w", err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	buffer := bufio.NewWriter(file)
	if err := write(json.NewEncoder(buffer)); err != nil {
		return "", fmt.Errorf("write TextRank counting run: %w", err)
	}
	if err := buffer.Flush(); err != nil {
		return "", fmt.Errorf("flush TextRank counting run: %w", err)
	}
	return path, nil
}

func (s *textRankCountStore) flush() error {
	if len(s.pending) == 0 {
		return nil
	}
	records := make([]textRankCountRecord, 0, len(s.pending))
	for key, count := range s.pending {
		records = append(records, textRankCountRecord{key.kind, key.entry, count})
	}
	slices.SortFunc(records, compareTextRankCounts)
	path, err := s.writeRun(func(encoder *json.Encoder) error {
		for _, record := range records {
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.pending = make(map[textRankCountKey]int)
	s.bytes = 0
	// Binary compaction avoids retaining an unbounded list of tiny runs or
	// repeatedly merging the entire corpus for every new batch.
	for level := 0; ; level++ {
		if level == len(s.runs) {
			s.runs = append(s.runs, path)
			return nil
		}
		if s.runs[level] == "" {
			s.runs[level] = path
			return nil
		}
		merged, err := s.merge(s.runs[level], path)
		if err != nil {
			return err
		}
		s.runs[level] = ""
		path = merged
	}
}

func (s *textRankCountStore) merge(left, right string) (path string, err error) {
	a, err := os.Open(left)
	if err != nil {
		return "", fmt.Errorf("open TextRank counting run: %w", err)
	}
	defer func() { err = errors.Join(err, a.Close()) }()
	b, err := os.Open(right)
	if err != nil {
		return "", fmt.Errorf("open TextRank counting run: %w", err)
	}
	defer func() { err = errors.Join(err, b.Close()) }()
	path, err = s.writeRun(func(encoder *json.Encoder) error {
		decoders := [2]*json.Decoder{
			json.NewDecoder(bufio.NewReader(a)), json.NewDecoder(bufio.NewReader(b)),
		}
		var records [2]textRankCountRecord
		var done [2]bool
		advance := func(i int) error {
			err := decoders[i].Decode(&records[i])
			if err == io.EOF {
				done[i] = true
				return nil
			}
			return err
		}
		if err := advance(0); err != nil {
			return err
		}
		if err := advance(1); err != nil {
			return err
		}
		for !done[0] || !done[1] {
			i := 0
			if done[0] || (!done[1] && compareTextRankCounts(records[0], records[1]) > 0) {
				i = 1
			}
			record := records[i]
			equal := !done[0] && !done[1] && compareTextRankCounts(records[0], records[1]) == 0
			if equal {
				record.Count += records[1-i].Count
			}
			if err := encoder.Encode(record); err != nil {
				return err
			}
			if err := advance(i); err != nil {
				return err
			}
			if equal {
				if err := advance(1 - i); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	for _, input := range []string{left, right} {
		if err := os.Remove(input); err != nil {
			return "", fmt.Errorf("remove merged TextRank counting run: %w", err)
		}
	}
	return path, nil
}

func (s *textRankCountStore) load(pruneThreshold, phraseThreshold float64) (counts *textRankEntryCounts, err error) {
	if err := s.flush(); err != nil {
		return nil, err
	}
	path := ""
	for _, run := range s.runs {
		if run == "" {
			continue
		}
		if path == "" {
			path = run
		} else {
			path, err = s.merge(path, run)
			if err != nil {
				return nil, err
			}
		}
	}
	counts = &textRankEntryCounts{
		phrases: make(map[string]int), words: make(map[string]int),
		similarityPhrases: make(map[string]int), subjects: make(map[string]int),
	}
	if path == "" {
		return counts, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open final TextRank counts: %w", err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	decoder := json.NewDecoder(bufio.NewReader(file))
	for {
		var record textRankCountRecord
		if err := decoder.Decode(&record); err != nil {
			if err == io.EOF {
				return counts, nil
			}
			return nil, fmt.Errorf("decode TextRank counts: %w", err)
		}
		var destination map[string]int
		switch record.Kind {
		case "phrase":
			if float64(record.Count) > pruneThreshold {
				destination = counts.phrases
			}
		case "word":
			if float64(record.Count) > pruneThreshold {
				destination = counts.words
			}
		case "similarity":
			if record.Entry != "" && record.Count >= similarityMinPhraseDocumentFrequency && float64(record.Count) <= phraseThreshold {
				destination = counts.similarityPhrases
			}
		case "subject":
			destination = counts.subjects
		default:
			return nil, fmt.Errorf("invalid TextRank count kind %q", record.Kind)
		}
		if destination != nil {
			destination[record.Entry] = record.Count
		}
	}
}

func (s *textRankCountStore) close() {
	if err := os.RemoveAll(s.dir); err != nil {
		log.Printf("Remove temporary TextRank counts: %s", err)
	}
}
