package infrastructure

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gofiber/template/html/v3"
	"github.com/gosimple/slug"
	"github.com/svera/coreander/v5/internal/i18n"
)

// TemplateEngine builds the html/template engine. assetVersion is a cache-busting
// token for static assets (CSS/JS/images), independent of the application's release
// version: a dev/dirty build's release version doesn't change between rebuilds
// unless committed, so tying cache-busting to it can leave a browser (mobile in
// particular, which has no "disable cache" escape hatch) stuck serving a stale
// immutable asset across multiple rebuilds of the same commit.
func TemplateEngine(viewsFS fs.FS, translator i18n.Translator, assetVersion string) (*html.Engine, error) {
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

	// Bound once at engine construction instead of threaded through every template's
	// data (which would mean adding it to every "dict" call building a partial's
	// isolated context, and easy to miss one), since it's process-wide and never
	// varies per request or per page.
	engine.AddFunc("assetVersion", func() string {
		return assetVersion
	})

	engine.AddFunc("versionParam", func() string {
		return "?v=" + assetVersion
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
			"eu": "Euskera",
			"gl": "Galego",
		}
		if name, ok := languageNames[code]; ok {
			return name
		}
		return strings.ToUpper(code)
	})

	engine.AddFunc("urlquery", func(text string) string {
		return url.QueryEscape(text)
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
