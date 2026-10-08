package index

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

type fileHash struct {
	ETag    string
	Size    int64
	ModTime int64
}

func fileHashKey(id string) []byte {
	return []byte("document-content-hash:" + id)
}

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
	key := fileHashKey(doc.ID)
	b.documentsMu.RLock()
	stored, err := b.documentsIdx.GetInternal(key)
	b.documentsMu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("read document hash %s: %w", doc.ID, err)
	}
	var hash fileHash
	if len(stored) > 0 {
		if err := json.Unmarshal(stored, &hash); err != nil {
			return nil, fmt.Errorf("decode document hash %s: %w", doc.ID, err)
		}
	}
	current := hash.ETag != "" && hash.Size == info.Size() && hash.ModTime == info.ModTime().UnixNano()
	if current && ifNoneMatch == hash.ETag {
		result := newIndexedFile(doc, nil)
		result.ETag = hash.ETag
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
	hash = fileHash{ETag: fmt.Sprintf(`"%x"`, sha256.Sum256(data)), Size: after.Size(), ModTime: after.ModTime().UnixNano()}
	encoded, err := json.Marshal(hash)
	if err != nil {
		return nil, fmt.Errorf("encode document hash %s: %w", doc.ID, err)
	}
	if !bytes.Equal(encoded, stored) {
		b.documentsMu.Lock()
		err = b.documentsIdx.SetInternal(key, encoded)
		b.documentsMu.Unlock()
		if err != nil {
			return nil, fmt.Errorf("save document hash %s: %w", doc.ID, err)
		}
	}
	result := newIndexedFile(doc, data)
	result.ETag = hash.ETag
	if ifNoneMatch == hash.ETag {
		result.Data = nil
	}
	return result, nil
}
