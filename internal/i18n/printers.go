package i18n

import (
	"io/fs"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

func Printers(dir fs.FS) (map[string]*message.Printer, error) {
	cat, err := NewCatalogFromFolder(dir, "en")
	if err != nil {
		return nil, err
	}

	message.DefaultCatalog = cat

	return map[string]*message.Printer{
		"en": message.NewPrinter(language.English),
		"es": message.NewPrinter(language.Spanish),
	}, nil
}
