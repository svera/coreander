package index_test

import (
	"maps"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/svera/coreander/v5/internal/index"
)

func newAuthorsTestIndex(t *testing.T) *index.BleveIndexer {
	t.Helper()
	documents, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authors, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	idx := index.NewBleve(documents, authors, nil, "", nil, index.Config{})
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

func TestStoreAuthorBio(t *testing.T) {
	idx := newAuthorsTestIndex(t)
	if err := idx.IndexAuthor(index.Author{Slug: "jane-doe", Name: "Jane Doe", DocumentCount: 1}); err != nil {
		t.Fatal(err)
	}

	original := index.AuthorBio{Source: "1.Jane_Doe", Language: "en", Text: map[string]string{"en": "Line one.\n\nLine two."}}
	if err := idx.StoreAuthorBio("jane-doe", original); err != nil {
		t.Fatal(err)
	}
	translation := index.AuthorBio{Source: original.Source, Language: "en", Text: map[string]string{"es": "Línea uno.\n\nLínea dos."}}
	if err := idx.StoreAuthorBio("jane-doe", translation); err != nil {
		t.Fatal(err)
	}
	author, err := idx.Author("jane-doe", "es")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"en": "Line one.\n\nLine two.", "es": "Línea uno.\n\nLínea dos."}
	if author.Bio.Source != original.Source || author.Bio.Language != "en" || !maps.Equal(author.Bio.Text, want) {
		t.Fatalf("translations must be merged with the original bio, got %+v", author.Bio)
	}

	author.Name = "Jane Doe Updated"
	author.Bio = index.AuthorBio{}
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	author, err = idx.Author("jane-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	if author.Name != "Jane Doe Updated" || !maps.Equal(author.Bio.Text, want) {
		t.Fatalf("IndexAuthor must preserve the stored bio, got %+v", author)
	}

	replacement := index.AuthorBio{Source: "2.Jane_Doe", Language: "fr", Text: map[string]string{"fr": "Bonjour."}}
	if err := idx.StoreAuthorBio("jane-doe", replacement); err != nil {
		t.Fatal(err)
	}
	author, err = idx.Author("jane-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	if author.Bio.Source != replacement.Source || author.Bio.Language != "fr" || !maps.Equal(author.Bio.Text, replacement.Text) {
		t.Fatalf("a bio from another source must replace the stored one, got %+v", author.Bio)
	}

	empty := index.AuthorBio{Source: "3.Jane_Doe"}
	if err := idx.StoreAuthorBio("jane-doe", empty); err != nil {
		t.Fatal(err)
	}
	author, err = idx.Author("jane-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	if author.Bio.Source != empty.Source || author.Bio.Language != "" || len(author.Bio.Text) != 0 {
		t.Fatalf("an empty bio must be stored to avoid new lookups, got %+v", author.Bio)
	}
}

func TestStoreAuthorBioIgnoresMissingAuthors(t *testing.T) {
	idx := newAuthorsTestIndex(t)
	if err := idx.StoreAuthorBio("missing", index.AuthorBio{Source: "1.Missing"}); err != nil {
		t.Fatal(err)
	}
	author, err := idx.Author("missing", "en")
	if err != nil {
		t.Fatal(err)
	}
	if author.Slug != "" {
		t.Fatalf("storing a bio must not create authors, got %+v", author)
	}
}

func TestReplaceAuthorBio(t *testing.T) {
	idx := newAuthorsTestIndex(t)
	if err := idx.IndexAuthor(index.Author{Slug: "jane-doe", Name: "Jane Doe", DocumentCount: 1}); err != nil {
		t.Fatal(err)
	}

	stale := index.AuthorBio{Source: "1.Jane_Doe", Language: "en", Text: map[string]string{"en": "Old.", "es": "Viejo."}}
	if err := idx.StoreAuthorBio("jane-doe", stale); err != nil {
		t.Fatal(err)
	}
	fresh := index.AuthorBio{Source: stale.Source, Language: "en", Text: map[string]string{"en": "New."}}
	if err := idx.ReplaceAuthorBio("jane-doe", fresh); err != nil {
		t.Fatal(err)
	}
	author, err := idx.Author("jane-doe", "en")
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(author.Bio.Text, fresh.Text) {
		t.Fatalf("replacing a bio must discard stored texts from the same source, got %+v", author.Bio)
	}
}

func TestStoreCanonicalAuthorBio(t *testing.T) {
	idx := newAuthorsTestIndex(t)
	const variant = "18647721.Plat_n"
	const linkKey = "external-profile"
	author := index.Author{Slug: "platon", Name: "Platón", DataSourceReferences: map[string]string{
		linkKey: variant, "website": "https://example.com",
	}}
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	bio := index.AuthorBio{Source: "879.Plato", Language: "en", Text: map[string]string{"en": "Primary bio"}}
	if err := idx.StoreCanonicalAuthorBio(author.Slug, linkKey, variant, bio); err != nil {
		t.Fatal(err)
	}
	stored, err := idx.Author(author.Slug, "en")
	if err != nil {
		t.Fatal(err)
	}
	if stored.DataSourceReferences[linkKey] != bio.Source || stored.Bio.Source != bio.Source || !maps.Equal(stored.Bio.Text, bio.Text) || stored.DataSourceReferences["website"] != "https://example.com" || stored.Name != author.Name {
		t.Fatalf("canonical resolution must update only the profile link and bio: %+v", stored)
	}
	if err := idx.StoreCanonicalAuthorBio(author.Slug, linkKey, variant, index.AuthorBio{Source: "999"}); err == nil {
		t.Fatal("a stale resolution must not overwrite a changed author link")
	}
	if err := idx.StoreCanonicalAuthorBio(author.Slug, linkKey, bio.Source, index.AuthorBio{}); err != nil {
		t.Fatal(err)
	}
	if err := idx.StoreAuthorBioForLink(author.Slug, linkKey, bio.Source, bio, false); err == nil {
		t.Fatal("a stale biography must not restore a cleared source")
	}
	stored, err = idx.Author(author.Slug, "en")
	if err != nil || stored.DataSourceReferences[linkKey] != "" || stored.DataSourceReferences["website"] != "https://example.com" || stored.Bio.Source != "" {
		t.Fatalf("clearing the profile link must preserve other author data: %+v, %v", stored, err)
	}
}
