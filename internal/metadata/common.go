package metadata

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// allowedDescriptionElements is the set of HTML elements permitted in rich descriptions.
var allowedDescriptionElements = []string{
	"p", "br", "strong", "em", "i", "b", "u", "s", "a", "blockquote", "cite", "code", "pre",
	"ol", "ul", "li", "h2", "h3", "h4", "h5", "h6", "dd", "dt", "dl", "dfn", "kbd", "mark",
	"q", "samp", "small", "sub", "sup", "time", "tt", "var",
}

// SanitizeDescription returns sanitized HTML for use in Metadata.Description.
// If raw is empty or only whitespace, returns "".
// If raw contains no HTML (strict sanitize unchanged), wraps newline-separated paragraphs in <p>.
// Otherwise sanitizes with a policy that allows allowedDescriptionElements.
func SanitizeDescription(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	strict := bluemonday.StrictPolicy()
	noHTML := strict.Sanitize(raw)
	if noHTML == raw {
		paragraphs := strings.Split(raw, "\n")
		return "<p>" + strings.Join(paragraphs, "</p><p>") + "</p>"
	}
	p := bluemonday.NewPolicy()
	p.AllowElements(allowedDescriptionElements...)
	return p.Sanitize(raw)
}

// DefaultTitleFromFilename returns the basename of fullPath without its extension (fallback book title).
func DefaultTitleFromFilename(fullPath string) string {
	return strings.TrimSuffix(filepath.Base(fullPath), filepath.Ext(fullPath))
}

// AuthorsOrEmptySlot returns authors unchanged if non-empty, otherwise a single empty string
// so Metadata always has at least one author slot (same convention as EPUB/CBZ readers).
func AuthorsOrEmptySlot(authors []string) []string {
	if len(authors) == 0 {
		return []string{""}
	}
	return authors
}

// ParseAuthorList splits s by '&', ',', and ';', trims each part, and returns non-empty names.
// Used for both EPUB creator lists and PDF author strings.
func ParseAuthorList(s string) []string {
	var names []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return r == '&' || r == ',' || r == ';'
	}) {
		if name := strings.TrimSpace(part); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// ParseSeriesIndex parses a series index or issue number string (e.g. EPUB calibre:series_index, CBZ Number).
// Returns 0 for empty or invalid input.
func ParseSeriesIndex(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return n
}

// ParseSubjectList splits s by ',' and ';', trims each part, and returns non-empty strings.
// Used for EPUB subjects and CBZ genre (and similar comma/semicolon-separated fields).
func ParseSubjectList(s string) []string {
	var list []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';'
	}) {
		if v := strings.TrimSpace(part); v != "" {
			list = append(list, v)
		}
	}
	return list
}
