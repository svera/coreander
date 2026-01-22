package document

import (
	"fmt"
	"html"
	"log"
	"net/mail"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/i18n"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func (d *Controller) Share(c *fiber.Ctx) error {
	slug := strings.TrimSpace(c.Params("slug"))
	if slug == "" {
		return fiber.ErrBadRequest
	}

	recipientsRaw := strings.TrimSpace(c.FormValue("recipients"))
	if recipientsRaw == "" {
		return fiber.ErrBadRequest
	}

	recipients := uniqueRecipients(splitRecipients(recipientsRaw))
	if len(recipients) == 0 {
		return fiber.ErrBadRequest
	}

	for _, recipient := range recipients {
		if _, err := mail.ParseAddress(recipient); err != nil {
			return fiber.ErrBadRequest
		}
	}

	document, err := d.idx.Document(slug)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	if document.Slug == "" {
		return fiber.ErrNotFound
	}

	session, _ := c.Locals("Session").(model.Session)
	lang, _ := c.Locals("Lang").(string)
	senderName := strings.TrimSpace(session.Name)
	if senderName == "" {
		senderName = strings.TrimSpace(session.Username)
	}
	if senderName == "" {
		senderName = strings.TrimSpace(session.Email)
	}
	if senderName == "" {
		senderName = d.translator.T(lang, "Someone")
	}

	docURL := fmt.Sprintf("%s/documents/%s", c.BaseURL(), document.Slug)
	subject := d.translator.T(lang, "%s shared \"%s\"", senderName, document.Title)
	body := shareBody(d.translator, lang, senderName, document.Title, docURL, c.FormValue("comment"))

	for _, recipient := range recipients {
		if err := d.sender.Send(recipient, subject, body); err != nil {
			log.Printf("error sending share to %s: %v\n", recipient, err)
			return fiber.ErrInternalServerError
		}
	}

	if session.ID > 0 {
		recipientIDs := make([]int, 0, len(recipients))
		for _, recipient := range recipients {
			user, err := d.usersRepository.FindByEmail(recipient)
			if err != nil {
				log.Printf("error finding share recipient %s: %v\n", recipient, err)
				return fiber.ErrInternalServerError
			}
			if user == nil {
				return fiber.ErrBadRequest
			}
			recipientIDs = append(recipientIDs, int(user.ID))
		}

		userID := int(session.ID)
		docSlug := document.Slug
		share := &model.Share{
			UserID:       &userID,
			DocumentSlug: &docSlug,
			Comment:      strings.TrimSpace(c.FormValue("comment")),
		}
		if err := d.shareRepository.CreateWithRecipients(share, recipientIDs); err != nil {
			log.Printf("error saving share: %v\n", err)
			return fiber.ErrInternalServerError
		}
	}

	return c.SendStatus(fiber.StatusOK)
}

func splitRecipients(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t'
	})
}

func uniqueRecipients(recipients []string) []string {
	unique := make(map[string]struct{}, len(recipients))
	result := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		trimmed := strings.TrimSpace(recipient)
		if trimmed == "" {
			continue
		}
		if _, ok := unique[trimmed]; ok {
			continue
		}
		unique[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func shareBody(translator i18n.Translator, lang, senderName, title, url, comment string) string {
	escapedSender := html.EscapeString(senderName)
	escapedTitle := html.EscapeString(title)
	escapedURL := html.EscapeString(url)
	escapedComment := html.EscapeString(strings.TrimSpace(comment))

	recommendedText := html.EscapeString(translator.T(lang, "shared a document:"))
	commentLabel := html.EscapeString(translator.T(lang, "Comment"))

	builder := &strings.Builder{}
	builder.WriteString("<p>")
	if escapedSender != "" {
		builder.WriteString(escapedSender)
		builder.WriteString(" ")
	}
	builder.WriteString(recommendedText)
	builder.WriteString("</p>")
	builder.WriteString("<p><a href=\"")
	builder.WriteString(escapedURL)
	builder.WriteString("\">")
	builder.WriteString(escapedTitle)
	builder.WriteString("</a></p>")
	if escapedComment != "" {
		builder.WriteString("<p>")
		builder.WriteString(commentLabel)
		builder.WriteString(":</p><p>")
		builder.WriteString(escapedComment)
		builder.WriteString("</p>")
	}
	return builder.String()
}
