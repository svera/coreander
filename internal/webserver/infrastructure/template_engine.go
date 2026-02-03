package infrastructure

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gofiber/template/html/v2"
	"github.com/gosimple/slug"
	"github.com/svera/coreander/v4/internal/i18n"
)

func TemplateEngine(viewsFS fs.FS, translator i18n.Translator) (*html.Engine, error) {
	engine := html.NewFileSystem(http.FS(viewsFS), ".html")

	engine.AddFunc("t", func(lang, key string, values ...any) template.HTML {
		return template.HTML(translator.T(lang, key, values...))
	})

	engine.AddFunc("language", func(lang string) template.HTML {
		if lang == "en" {
			return template.HTML("English")
		}
		return template.HTML(translator.T(lang, "_language"))
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

	engine.AddFunc("versionParam", func(version string) string {
		if version != "" && version != "unknown" {
			return "?v=" + version
		}
		return ""
	})

	engine.AddFunc("languageName", func(code string) string {
		languageNames := map[string]string{
			"en": "English",
			"es": "Español",
			"fr": "Français",
			"de": "Deutsch",
			"it": "Italiano",
			"pt": "Português",
			"nl": "Nederlands",
			"ru": "Русский",
			"ja": "日本語",
			"zh": "中文",
			"ko": "한국어",
			"ar": "العربية",
			"hi": "हिन्दी",
			"pl": "Polski",
			"tr": "Türkçe",
			"sv": "Svenska",
			"no": "Norsk",
			"da": "Dansk",
			"fi": "Suomi",
			"cs": "Čeština",
			"ro": "Română",
			"hu": "Magyar",
			"el": "Ελληνικά",
			"he": "עברית",
			"th": "ไทย",
			"vi": "Tiếng Việt",
			"id": "Bahasa Indonesia",
			"ms": "Bahasa Melayu",
			"uk": "Українська",
			"ca": "Català",
			"bg": "Български",
			"hr": "Hrvatski",
			"sk": "Slovenčina",
			"sl": "Slovenščina",
			"lt": "Lietuvių",
			"lv": "Latviešu",
			"et": "Eesti",
		}
		if name, ok := languageNames[code]; ok {
			return name
		}
		return strings.ToUpper(code)
	})

	engine.AddFunc("urlquery", func(text string) string {
		return url.QueryEscape(text)
	})

	engine.AddFunc("printf", func(format string, values ...any) string {
		return fmt.Sprintf(format, values...)
	})

	engine.AddFunc("sprintfHTML", func(format interface{}, values ...any) template.HTML {
		formatStr := ""
		switch v := format.(type) {
		case string:
			formatStr = v
		case template.HTML:
			formatStr = string(v)
		default:
			formatStr = fmt.Sprintf("%v", v)
		}
		return template.HTML(fmt.Sprintf(formatStr, values...))
	})

	return engine, nil
}

func notLast[V any](slice []V, index int) bool {
	return index < len(slice)-1
}
