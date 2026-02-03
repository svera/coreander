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
	"golang.org/x/exp/slices"
)

func (d *Controller) Share(c *fiber.Ctx) error {
	// Check if email sending is configured
	if _, ok := d.sender.(*infrastructure.NoEmail); ok {
		return fiber.ErrNotFound
	}

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
	if session.PrivateProfile != 0 {
		return fiber.ErrForbidden
	}
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

	recipientUsers := make([]*model.User, 0, len(recipients))
	recipientUserIDs := make(map[uint]struct{}, len(recipients))

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
	}

	if len(recipientUsers) == 0 {
		return fiber.ErrBadRequest
	}

	comment := strings.TrimSpace(c.FormValue("comment"))
	if len(comment) > 280 {
		comment = string([]rune(comment)[:280])
	}

	shareAlreadyExists := false
	if session.ID > 0 {
		recipientIDs := make([]int, 0, len(recipientUsers))
		for _, user := range recipientUsers {
			recipientIDs = append(recipientIDs, int(user.ID))
		}

		if err := d.hlRepository.Share(int(session.ID), document.ID, document.Slug, comment, recipientIDs); err != nil {
			if errors.Is(err, model.ErrShareAlreadyExists) {
				shareAlreadyExists = true
			} else {
				log.Printf("error saving share: %v\n", err)
				return fiber.ErrInternalServerError
			}
		}
	}

	if !shareAlreadyExists {
		if _, ok := d.sender.(*infrastructure.NoEmail); !ok {
			supportedLanguages := d.translator.SupportedLanguages()
			// Group recipients by language
			recipientsByLang := make(map[string][]*model.User)
			for _, recipientUser := range recipientUsers {
				// Use recipient's language preference, fallback to "en" if not set or not supported
				recipientLang := recipientUser.Language
				if recipientLang == "" || !slices.Contains(supportedLanguages, recipientLang) {
					recipientLang = "en"
				}
				recipientsByLang[recipientLang] = append(recipientsByLang[recipientLang], recipientUser)
			}

			// Send one BCC email per language group
			for recipientLang, langRecipients := range recipientsByLang {
				subject := d.translator.T(recipientLang, "%s shared \"%s\"", senderName, document.Title)

				// Render email template in this language
				if err := c.Render("document/share-email", fiber.Map{
					"Lang":          recipientLang,
					"SenderName":    senderName,
					"DocumentTitle": document.Title,
					"DocumentURL":   docURL,
					"HighlightsURL": highlightsURL,
					"Comment":       comment,
				}); err != nil {
					log.Printf("error rendering email: %v\n", err)
					return fiber.ErrInternalServerError
				}

				body := string(c.Response().Body())
				// Collect all email addresses for this language group
				addresses := make([]string, 0, len(langRecipients))
				for _, recipientUser := range langRecipients {
					addresses = append(addresses, recipientUser.Email)
				}

				if err := d.sender.SendBCC(addresses, subject, body); err != nil {
					log.Printf("error sending share email: %v\n", err)
					return fiber.ErrInternalServerError
				}
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
		user, err := d.usersRepository.FindByEmail(strings.TrimSpace(address.Address))
		if err != nil || user == nil || user.PrivateProfile != 0 {
			return nil, err
		}
		return user, nil
	}

	user, err := d.usersRepository.FindByUsername(trimmed)
	if err != nil || user == nil || user.PrivateProfile != 0 {
		return nil, err
	}
	return user, nil
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
