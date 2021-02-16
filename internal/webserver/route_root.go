package webserver

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/text/language"
)

func rootRoute(c *fiber.Ctx) error {
	acceptHeader := c.Get(fiber.HeaderAcceptLanguage)
	languageMatcher := language.NewMatcher([]language.Tag{
		language.English,
		language.Spanish,
	})

	t, _, _ := language.ParseAcceptLanguage(acceptHeader)
	tag, _, _ := languageMatcher.Match(t...)
	baseLang, _ := tag.Base()
	return c.Redirect(fmt.Sprintf("/%s", baseLang.String()))
}
