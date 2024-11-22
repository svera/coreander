package highlight

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func (h *Controller) Delete(c *fiber.Ctx) error {
	/*emailSendingConfigured := true
	if _, ok := h.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}*/

	user := c.Locals("Session").(model.Session)

	document, err := h.idx.Document(c.Params("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err = h.hlRepository.Remove(int(user.ID), document.ID); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	/*		docsSortedByHighlightedDate, err := h.hlRepository.Highlights(int(user.ID), 0, latestHighlightsAmount)
			if err != nil {
				log.Println(err)
				return fiber.ErrInternalServerError
			}

			docs, err := h.idx.Documents(docsSortedByHighlightedDate.Hits())
			if err != nil {
				log.Println(err)
				return fiber.ErrInternalServerError
			}

			highlights := make([]index.Document, 0, len(docs))
			for _, path := range docsSortedByHighlightedDate.Hits() {
				if _, ok := docs[path]; !ok {
					continue
				}
				doc := docs[path]
				doc.Highlighted = true
				highlights = append(highlights, doc)
			}

			err = c.Render("partials/latest-highlights", fiber.Map{
				"Highlights":             highlights,
				"EmailSendingConfigured": emailSendingConfigured,
				"EmailFrom":              h.sender.From(),
				"WordsPerMinute":         h.wordsPerMinute,
			})
			if err != nil {
				log.Println(err)
				return fiber.ErrInternalServerError
			}
	*/

	c.Response().Header.Set("HX-Trigger", "dehighlight")
	return nil
}
