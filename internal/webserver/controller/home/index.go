package home

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
)

func (d *Controller) Index(c *fiber.Ctx) error {
	emailSendingConfigured := true
	if _, ok := d.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	count, err := d.idx.Count(index.TypeDocument)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return c.Render("index", fiber.Map{
		"Count":                  count,
		"Title":                  "Home",
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              d.sender.From(),
		"HomeNavbar":             true,
	}, "layout")
}
