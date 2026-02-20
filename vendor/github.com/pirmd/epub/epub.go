// Package epub provides a way to retrieve stored metadata from epub files.
package epub

import (
	"archive/zip"
	"io/fs"
	"net/url"
	"path/filepath"
)

// Epub represents a read-only EPUB document.
type Epub struct {
	*zip.ReadCloser

	rootfile string
}

// Open an EPUB from a file.
// Returned Epub needs to be closed when no longer needed.
func Open(path string) (*Epub, error) {
	e := new(Epub)

	var err error
	if e.ReadCloser, err = zip.OpenReader(path); err != nil {
		return nil, err
	}

	c, err := e.container()
	if err != nil {
		e.Close()
		return nil, err
	}

	e.rootfile = c.Rootfiles.FullPath

	return e, nil
}

// OpenItem opens an EPUB Publication Resource identified by its href as
// usually found in Manifest.
// OpenItem will try to unescape href first.
// Opening Items whoses Href points outside of EPUB archive will failed.
func (e *Epub) OpenItem(href string) (fs.File, error) {
	name, err := url.PathUnescape(href)
	if err != nil {
		return nil, err
	}

	path := filepath.Join(filepath.Dir(e.rootfile), name)
	return e.Open(path)
}

// container returns the EPUB Container.
func (e *Epub) container() (*container, error) {
	r, err := e.Open(containerPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return newContainer(r)
}

// Package returns the EPUB PackageDocument.
func (e *Epub) Package() (*PackageDocument, error) {
	r, err := e.Open(e.rootfile)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return newPackageDocument(r)
}

// Information returns a simplified but easier to use version of
// PackageDocument.Metadata.
func (e *Epub) Information() (*Information, error) {
	opf, err := e.Package()
	if err != nil {
		return nil, err
	}

	return getMeta(opf.Metadata), nil
}

// GetPackageFromFile reads an epub's Open Package Document from an epub  file.
func GetPackageFromFile(path string) (*PackageDocument, error) {
	e, err := Open(path)
	if err != nil {
		return nil, err
	}
	defer e.Close()

	return e.Package()
}
