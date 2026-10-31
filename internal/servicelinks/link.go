// Package servicelinks provides external author and document page links.
package servicelinks

// Link has an empty URL when the service has no known page for the entity.
type Link struct {
	Source string
	URL    string
	Icon   string
}
