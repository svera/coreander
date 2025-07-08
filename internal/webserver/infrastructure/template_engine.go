package infrastructure

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gofiber/template/html/v2"
	"github.com/gosimple/slug"
	"golang.org/x/text/message"
)

func TemplateEngine(viewsFS fs.FS, printers map[string]*message.Printer) (*html.Engine, error) {
	engine := html.NewFileSystem(http.FS(viewsFS), ".html")

	engine.AddFunc("t", func(lang, key string, values ...any) template.HTML {
		return template.HTML(printers[lang].Sprintf(key, values...))
	})

	engine.AddFunc("dict", func(values ...any) map[string]any {
		if len(values)%2 != 0 {
			fmt.Println("invalid dict call")
			return nil
		}
		dict := make(map[string]any, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				fmt.Println("dict keys must be strings")
				return nil
			}
			dict[key] = values[i+1]
		}
		return dict
	})

	engine.AddFunc("uppercase", func(text string) string {
		return strings.ToUpper(text)
	})

	engine.AddFunc("notLast", notLast[string])

	engine.AddFunc("basename", func(path string) string {
		return filepath.Base(path)
	})

	engine.AddFunc("join", func(elems []string, sep string) string {
		return strings.Join(elems, sep)
	})

	engine.AddFunc("slugify", func(text string) string {
		return slug.Make(text)
	})

	return engine, nil
}

func notLast[V any](slice []V, index int) bool {
	return index < len(slice)-1
}
