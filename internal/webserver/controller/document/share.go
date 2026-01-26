package document

import (
	"errors"
	"fmt"
	"log"
	"net/mail"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
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
	highlightsURL := fmt.Sprintf("%s/highlights", c.BaseURL())
	subject := d.translator.T(lang, "%s shared \"%s\"", senderName, document.Title)

	recipientUsers := make([]*model.User, 0, len(recipients))
	recipientUserIDs := make(map[uint]struct{}, len(recipients))
	recipientEmails := make([]string, 0, len(recipients))

	for _, recipient := range recipients {
		user, err := d.resolveRecipient(recipient)
		if err != nil {
			return fiber.ErrBadRequest
		}
		if user == nil {
			return fiber.ErrBadRequest
		}
		if _, seen := recipientUserIDs[user.ID]; seen {
			continue
		}
		recipientUserIDs[user.ID] = struct{}{}
		recipientUsers = append(recipientUsers, user)
		recipientEmails = append(recipientEmails, user.Email)
	}

	if len(recipientUsers) == 0 {
		return fiber.ErrBadRequest
	}

	if session.ID > 0 {
		recipientIDs := make([]int, 0, len(recipientUsers))
		for _, user := range recipientUsers {
			recipientIDs = append(recipientIDs, int(user.ID))
		}

		if err := d.hlRepository.Share(int(session.ID), document.ID, document.Slug, strings.TrimSpace(c.FormValue("comment")), recipientIDs); err != nil {
			if errors.Is(err, model.ErrShareAlreadyExists) {
				return fiber.ErrBadRequest
			}
			log.Printf("error saving share: %v\n", err)
			return fiber.ErrInternalServerError
		}
	}

	if _, ok := d.sender.(*infrastructure.NoEmail); !ok {
		c.Render("document/share-email", fiber.Map{
			"Lang":          lang,
			"SenderName":    senderName,
			"DocumentTitle": document.Title,
			"DocumentURL":   docURL,
			"HighlightsURL": highlightsURL,
			"Comment":       strings.TrimSpace(c.FormValue("comment")),
		})
		body := string(c.Response().Body())
		for _, recipient := range recipientEmails {
			if err := d.sender.Send(recipient, subject, body); err != nil {
				log.Printf("error sending share to %s: %v\n", recipient, err)
				return fiber.ErrInternalServerError
			}
		}
	}

	return c.SendStatus(fiber.StatusOK)
}

func (d *Controller) resolveRecipient(recipient string) (*model.User, error) {
	trimmed := strings.TrimSpace(recipient)
	if trimmed == "" {
		return nil, nil
	}

	if strings.Contains(trimmed, "@") {
		address, err := mail.ParseAddress(trimmed)
		if err != nil {
			return nil, err
		}
		return d.usersRepository.FindByEmail(strings.TrimSpace(address.Address))
	}

	return d.usersRepository.FindByUsername(trimmed)
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

