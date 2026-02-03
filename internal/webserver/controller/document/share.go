package document

import (
	"fmt"
	"log"
	"net/mail"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/index"
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
	senderName := strings.TrimSpace(session.Name)
	if senderName == "" {
		senderName = strings.TrimSpace(session.Username)
	}

	docURL := fmt.Sprintf("%s/documents/%s", c.BaseURL(), document.Slug)
	highlightsURL := fmt.Sprintf("%s/highlights", c.BaseURL())

	recipientUsers, err := d.resolveRecipients(recipients)
	if err != nil {
		return err
	}

	if len(recipientUsers) == 0 {
		return fiber.ErrBadRequest
	}

	comment := strings.TrimSpace(c.FormValue("comment"))
	if len(comment) > 280 {
		comment = string([]rune(comment)[:280])
	}

	newRecipients := recipientUsers
	if session.ID > 0 {
		recipientIDs := make([]int, 0, len(recipientUsers))
		for _, user := range recipientUsers {
			recipientIDs = append(recipientIDs, int(user.ID))
		}

		// Filter out recipients who already have the document
		newRecipients = d.filterNewRecipients(recipientUsers, document.ID)
		if len(newRecipients) > 0 {
			newRecipientIDs := make([]int, 0, len(newRecipients))
			for _, user := range newRecipients {
				newRecipientIDs = append(newRecipientIDs, int(user.ID))
			}

			if err := d.hlRepository.Share(int(session.ID), document.ID, document.Slug, comment, newRecipientIDs); err != nil {
				log.Printf("error saving share: %v\n", err)
				return fiber.ErrInternalServerError
			}
		}
	}

	// Only send emails to recipients who actually received a new share
	if len(newRecipients) > 0 {
		if err := d.sendShareEmails(c, newRecipients, senderName, document.Title, docURL, highlightsURL, comment); err != nil {
			return err
		}
	}

	return nil
}

func (d *Controller) sendShareEmails(c *fiber.Ctx, recipientUsers []*model.User, senderName, documentTitle, docURL, highlightsURL, comment string) error {
	if _, ok := d.sender.(*infrastructure.NoEmail); ok {
		return nil
	}

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
		subject := d.translator.T(recipientLang, "%s shared \"%s\"", senderName, documentTitle)

		// Render email template in this language
		if err := c.Render("document/share-email", fiber.Map{
			"Lang":          recipientLang,
			"SenderName":    senderName,
			"DocumentTitle": documentTitle,
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

	return nil
}

func (d *Controller) resolveRecipients(recipients []string) ([]*model.User, error) {
	recipientUsers := make([]*model.User, 0, len(recipients))
	recipientUserIDs := make(map[uint]struct{}, len(recipients))

	for _, recipient := range recipients {
		user, err := d.resolveRecipient(recipient)
		if err != nil {
			return nil, fiber.ErrBadRequest
		}
		if user == nil {
			return nil, fiber.ErrBadRequest
		}
		if _, seen := recipientUserIDs[user.ID]; seen {
			continue
		}
		recipientUserIDs[user.ID] = struct{}{}
		recipientUsers = append(recipientUsers, user)
	}

	return recipientUsers, nil
}

func (d *Controller) filterNewRecipients(recipientUsers []*model.User, documentID string) []*model.User {
	if len(recipientUsers) == 0 {
		return recipientUsers
	}

	newRecipients := make([]*model.User, 0, len(recipientUsers))
	for _, user := range recipientUsers {
		// Check if user already has this document highlighted
		checkedDoc := d.hlRepository.Highlighted(int(user.ID), index.Document{ID: documentID})
		if !checkedDoc.Highlighted {
			newRecipients = append(newRecipients, user)
		}
	}

	return newRecipients
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
