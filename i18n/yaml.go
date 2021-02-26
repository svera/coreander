package i18n

import (
	"io/fs"
	"path/filepath"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message/catalog"
	"gopkg.in/yaml.v2"
)

type yamlDictionary struct {
	Entries map[string]string
}

func (d *yamlDictionary) Lookup(key string) (data string, ok bool) {
	if value, ok := d.Entries[key]; ok {
		// \x02 is ASCII code for hex 02, which is STX (start of text)
		return "\x02" + value, true
	}
	return "", false
}

// NewCatalogFromFolder read all translations yml files from dir and generates a
// translation catalog from them. Each yml file must be named as the two-letter
// identifier of the language of the translation, e. g. "es" for spanish, "en" for english, etc.
func NewCatalogFromFolder(dir fs.FS, fallbackLang string) (catalog.Catalog, error) {
	files, err := fs.ReadDir(dir, "embedded/translations")
	if err != nil {
		return nil, err
	}
	translations := map[string]catalog.Dictionary{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		yamlFile, err := fs.ReadFile(dir, "embedded/translations/"+file.Name())
		if err != nil {
			return nil, err
		}
		lang := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
		dict, err := ParseYAMLDict(yamlFile)
		if err != nil {
			return nil, err
		}
		translations[lang] = dict
	}
	fallback := language.MustParse(fallbackLang)
	cat, err := catalog.NewFromMap(translations, catalog.Fallback(fallback))
	if err != nil {
		return nil, err
	}
	return cat, err
}

func ParseYAMLDict(file []byte) (*yamlDictionary, error) {
	data := map[string]string{}
	err := yaml.Unmarshal(file, &data)
	if err != nil {
		return nil, err
	}
	return &yamlDictionary{Entries: data}, nil
}
