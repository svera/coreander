package document

import (
	"strings"

	"github.com/svera/coreander/v5/internal/index"
)

func documentPageTitle(document index.Document) (title, authors string) {
	authorNames := make([]string, 0, len(document.Authors))
	for _, author := range document.Authors {
		if name := strings.TrimSpace(author); name != "" {
			authorNames = append(authorNames, name)
		}
	}
	authors = strings.Join(authorNames, ", ")
	title = document.Title
	if authors != "" {
		title = authors + " - " + title
	}
	return title, authors
}
