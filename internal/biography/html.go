package biography

import (
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var paragraphEnd = regexp.MustCompile(`(?i)</p>`)
var htmlLineBreak = regexp.MustCompile(`(?i)<br\s*/?>|</div>`)
var htmlFormatting = bluemonday.NewPolicy().AllowElements("b", "strong", "i", "em")

// spaceBeforePunctuation matches whitespace left by inline elements, as in "<i>Title </i>,".
var spaceBeforePunctuation = regexp.MustCompile(` +([,.;:!?)\]])`)

// CleanHTML retains safe emphasis and converts HTML block boundaries to newlines.
func CleanHTML(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	text = paragraphEnd.ReplaceAllString(text, "\n\n")
	text = htmlLineBreak.ReplaceAllString(text, "\n")
	text = htmlFormatting.Sanitize(text)
	var lines []string
	blank := false
	for line := range strings.SplitSeq(text, "\n") {
		line = spaceBeforePunctuation.ReplaceAllString(strings.Join(strings.Fields(line), " "), "$1")
		if line == "" {
			blank = len(lines) > 0
			continue
		}
		if blank {
			lines = append(lines, "")
			blank = false
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
