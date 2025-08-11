package i18n

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/message/catalog"
)

type Printers struct {
	printers map[string]*message.Printer
}

func New(dir fs.FS, fallbackLang string) (*Printers, error) {
	cat, err := newCatalogFromFolder(dir, fallbackLang)
	if err != nil {
		return nil, err
	}

	message.DefaultCatalog = cat

	base, err := language.Parse(fallbackLang)
	if err != nil {
		return nil, err
	}

	printers := map[string]*message.Printer{
		fallbackLang: message.NewPrinter(base),
	}

	for _, lang := range cat.Languages() {
		base, _ := lang.Base()
		twoLetterCode := strings.Split(base.String(), "_")[0]
		printers[twoLetterCode] = message.NewPrinter(lang)
	}

	return &Printers{printers: printers}, nil
}

// T returns the translated string for the given key in the specified language.
func (p *Printers) T(lang, key string, values ...any) string {
	return p.printers[lang].Sprintf(key, values...)
}

// SupportedLanguages returns a sorted list of supported languages.
func (p *Printers) SupportedLanguages() []string {
	langs := make([]string, len(p.printers))

	i := 0
	for k := range p.printers {
		langs[i] = k
		i++
	}

	sort.Strings(langs)
	return langs
}

// newCatalogFromFolder read all translations yml files from dir and generates a
// translation catalog from them. Each yml file must be named as the two-letter
// identifier of the language of the translation, e. g. "es" for spanish, "en" for english, etc.
func newCatalogFromFolder(dir fs.FS, fallbackLang string) (catalog.Catalog, error) {
	files, err := fs.ReadDir(dir, ".")
	if err != nil {
		return nil, err
	}
	translations := map[string]catalog.Dictionary{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		translationContent, err := fs.ReadFile(dir, file.Name())
		if err != nil {
			return nil, err
		}
		lang := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
		dict, err := ParseDict(translationContent)
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
