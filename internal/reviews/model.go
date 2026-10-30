// Package reviews provides service-independent review data and service adapters.
package reviews

import "time"

// Score has an empty URL for an unmatched document and an empty Value for an unavailable score.
type Score struct {
	Source string
	Value  string
	Count  string
	URL    string
}

type Review struct {
	Reviewer string
	// Rating is on a five-point scale; zero means unrated.
	Rating int
	// Text may contain untrusted HTML, which must be sanitized before rendering.
	Text      string
	CreatedAt time.Time
	Spoiler   bool
	URL       string
}

// Page distinguishes an unavailable review section from an available, empty list.
type Page struct {
	Source    string
	URL       string
	Available bool
	Reviews   []Review
}
