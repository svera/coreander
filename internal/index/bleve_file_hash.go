package index

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/afero"
)

// File returns document metadata and its ETag, lazily persisting the hash in the
// index. An empty ifNoneMatch always returns bytes; a matching ETag omits them.
func (b *BleveIndexer) File(slug, ifNoneMatch string) (*IndexedFile, error) {
	doc, err := b.Document(slug)
	if err != nil {
		return nil, err
	}
	if doc.ID == "" {
		return nil, ErrDocumentNotFound
	}
	unlock := b.lockFile(doc.ID)
	defer unlock()
	doc, err = b.Document(slug)
	if err != nil {
		return nil, err
	}
	if doc.ID == "" {
		return nil, ErrDocumentNotFound
	}

	path := filepath.Join(b.libraryPath, doc.ID)
	info, err := b.fs.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrDocumentNotFound
		}
		return nil, fmt.Errorf("stat document %s: %w", doc.ID, err)
	}
	etag := ""
	if doc.ContentHash != "" {
		etag = `"` + doc.ContentHash + `"`
	}
	current := etag != "" && doc.ContentSize == info.Size() && doc.ContentModTime == info.ModTime().UTC().Format(time.RFC3339Nano)
	if current && ifNoneMatch == etag {
		result := newIndexedFile(doc, nil)
		result.ETag = etag
		return result, nil
	}

	data, err := afero.ReadFile(b.fs, path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrDocumentNotFound
		}
		return nil, fmt.Errorf("read document %s: %w", doc.ID, err)
	}
	after, err := b.fs.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat document after reading %s: %w", doc.ID, err)
	}
	if info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) || int64(len(data)) != after.Size() {
		return nil, fmt.Errorf("document %s changed while reading", doc.ID)
	}
	// Hash bytes already read for a full download, including changes that
	// happen to preserve the file's size and modification time.
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	modTime := after.ModTime().UTC().Format(time.RFC3339Nano)
	if hash != doc.ContentHash || after.Size() != doc.ContentSize || modTime != doc.ContentModTime {
		b.documentsMu.Lock()
		doc, err = b.documentByIndexIDLocked(doc.ID)
		if err == nil && doc.ID == "" {
			err = ErrDocumentNotFound
		}
		if err == nil {
			doc.ContentHash = hash
			doc.ContentSize = after.Size()
			doc.ContentModTime = modTime
			err = b.documentsIdx.Index(doc.ID, doc)
		}
		b.documentsMu.Unlock()
		if err != nil {
			return nil, fmt.Errorf("save document hash %s: %w", doc.ID, err)
		}
	}
	result := newIndexedFile(doc, data)
	result.ETag = `"` + hash + `"`
	if ifNoneMatch == result.ETag {
		result.Data = nil
	}
	return result, nil
}
