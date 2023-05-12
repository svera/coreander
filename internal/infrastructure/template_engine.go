package infrastructure

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gofiber/template/html"
	"golang.org/x/text/message"
)

func TemplateEngine(viewsFS fs.FS, printers map[string]*message.Printer) (*html.Engine, error) {
	engine := html.NewFileSystem(http.FS(viewsFS), ".html")

	engine.AddFunc("t", func(lang, key string, values ...interface{}) template.HTML {
		return template.HTML(printers[lang].Sprintf(key, values...))
	})

	engine.AddFunc("dict", func(values ...interface{}) map[string]interface{} {
		if len(values)%2 != 0 {
			fmt.Println("invalid dict call")
			return nil
		}
		dict := make(map[string]interface{}, len(values)/2)
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

	return engine, nil
}

func notLast[V any](slice []V, index int) bool {
	return index < len(slice)-1
}
