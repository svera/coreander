package controller

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/text/language"
)

func Root(c *fiber.Ctx, supportedLanguages []string) error {
	acceptHeader := c.Get(fiber.HeaderAcceptLanguage)
	tags := make([]language.Tag, len(supportedLanguages))
	for i, lang := range supportedLanguages {
		tags[i] = language.Make(lang)
	}
	languageMatcher := language.NewMatcher(tags)

	t, _, _ := language.ParseAcceptLanguage(acceptHeader)
	tag, _, _ := languageMatcher.Match(t...)
	baseLang, _ := tag.Base()
	return c.Redirect(fmt.Sprintf("/%s", baseLang.String()))
}
