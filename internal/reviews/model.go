// Package reviews provides service-independent review data and service adapters.
package reviews

import (
	"strconv"
	"strings"
	"time"
)

// Score has an empty URL for an unmatched document and an empty Value for an unavailable score.
type Score struct {
	Source string
	Value  string
	Count  string
	URL    string
}

// HasScore reports whether the value is scored or has valuations despite a zero value.
func (s Score) HasScore() bool {
	if s.Value == "" {
		return false
	}
	value, err := strconv.ParseFloat(s.Value, 64)
	if err != nil || value != 0 {
		return true
	}
	count, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s.Count), ",", ""), 64)
	return err == nil && count > 0
}

type Review struct {
	Reviewer string
	// Rating is on a five-point scale and may be fractional; zero means unrated.
	Rating float64
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
