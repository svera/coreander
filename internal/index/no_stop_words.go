package index

import (
	"fmt"

	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2/mapping"
)

var stemmers = map[string]string{
	"es": "stemmer_es_light",
	"en": "stemmer_porter",
	"de": "stemmer_de_light",
	"fr": "stemmer_fr_light",
	"it": "stemmer_it_light",
	"pt": "stemmer_pt_light",
}

func addNoStopWordsAnalyzer(lang string, indexMapping *mapping.IndexMappingImpl) error {
	if _, ok := stemmers[lang]; !ok {
		return fmt.Errorf("no stemmer defined for %s", lang)
	}

	err := indexMapping.AddCustomAnalyzer(lang+"_no_stop_words",
		map[string]interface{}{
			"type":      custom.Name,
			"tokenizer": unicode.Name,
			"token_filters": []string{
				lowercase.Name,
				stemmers[lang],
			},
		})

	return err
}
