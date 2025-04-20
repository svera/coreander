package index

import (
	"fmt"

	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2/mapping"
)

func addNoStopWordsAnalyzer(lang string, indexMapping *mapping.IndexMappingImpl) error {
	if _, ok := noStopWordsFilters[lang]; !ok {
		return fmt.Errorf("no stemmer defined for %s", lang)
	}

	return indexMapping.AddCustomAnalyzer(lang+"_no_stop_words",
		map[string]any{
			"type":          custom.Name,
			"tokenizer":     unicode.Name,
			"token_filters": noStopWordsFilters[lang],
		})
}
