package user

import (
	"fmt"
	"log"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// SendInvite sends invitation emails to one or more addresses (comma-separated).
func (u *Controller) SendInvite(c fiber.Ctx) error {
	if _, ok := u.sender.(*infrastructure.NoEmail); ok {
		return fiber.ErrNotFound
	}

	raw := c.FormValue("email")
	lang := c.Locals("Lang").(string)

	errs, err := u.validateAndPrepareInviteEmails(raw, lang)
	if err != nil {
		return err
	}

	if len(errs) > 0 {
		return c.Status(fiber.StatusUnprocessableEntity).Render("partials/user-invite-modal-form", fiber.Map{
			"Lang":                     lang,
			"InviteFormErrors":         errs,
			"InviteFormEmail":          raw,
			"InviteEmailListMaxLength": u.config.InviteEmailListMaxLength,
		})
	}

	addresses := parseCommaSeparatedInviteEmails(raw)

	fqdn := u.config.FQDN
	if !strings.HasPrefix(fqdn, "http://") && !strings.HasPrefix(fqdn, "https://") {
		fqdn = "http://" + fqdn
	}

	subject := u.translator.T(lang, "You've been invited to join Coreander")
	type inviteEmail struct {
		email string
		body  string
	}
	emails := make([]inviteEmail, 0, len(addresses))

	for _, email := range addresses {
		if err := u.invitationsRepository.DeleteByEmail(email); err != nil {
			log.Printf("error deleting old invitation: %v\n", err)
			return fiber.ErrInternalServerError
		}

		invitation := &model.Invitation{
			Email:      email,
			UUID:       uuid.NewString(),
			ValidUntil: time.Now().UTC().Add(u.config.InvitationTimeout),
		}
		if err := u.invitationsRepository.Create(invitation); err != nil {
			log.Printf("error creating invitation: %v\n", err)
			return fiber.ErrInternalServerError
		}

		invitationLink := fmt.Sprintf("%s/invite?id=%s", fqdn, invitation.UUID)
		if err := c.Render("user/invitation-email", fiber.Map{
			"InvitationLink":    invitationLink,
			"InvitationTimeout": strconv.FormatFloat(u.config.InvitationTimeout.Hours(), 'f', -1, 64),
		}); err != nil {
			log.Printf("error rendering invitation email: %v\n", err)
			return fiber.ErrInternalServerError
		}
		emails = append(emails, inviteEmail{
			email: email,
			body:  string(c.Response().Body()),
		})
	}

	for _, invite := range emails {
		go func(invite inviteEmail) {
			if err := u.sender.Send(invite.email, subject, invite.body); err != nil {
				log.Printf("error sending invitation email to %s: %v\n", invite.email, err)
			}
		}(invite)
	}

	var successMsg string
	if len(addresses) == 1 {
		successMsg = u.translator.T(lang, "Invitation sent successfully to %s", addresses[0])
	} else {
		successMsg = u.translator.T(lang, "%d invitations sent successfully", len(addresses))
	}

	if c.Get("HX-Request") == "true" {
		return c.SendStatus(fiber.StatusNoContent)
	}

	c.Cookie(&fiber.Cookie{
		Name:    "success-once",
		Value:   successMsg,
		Expires: time.Now().Add(24 * time.Hour),
	})
	return c.Redirect().To("/users")
}

// AcceptInviteForm shows the form to accept an invitation
func (u *Controller) AcceptInviteForm(c fiber.Ctx) error {
	invitation, err := u.validateInvitation(c.Query("id"))
	if err != nil {
		return err
	}

	return c.Render("user/accept-invite", fiber.Map{
		"Title":            "Accept invitation",
		"InvitationUUID":   invitation.UUID,
		"Email":            invitation.Email,
		"Errors":           map[string]string{},
		"DisableLoginLink": true,
	}, "layout")
}

// AcceptInvite processes the invitation acceptance
func (u *Controller) AcceptInvite(c fiber.Ctx) error {
	invitation, err := u.validateInvitation(c.FormValue("invitation_uuid"))
	if err != nil {
		return err
	}

	lang := c.Locals("Lang").(string)

	// Create user from form data with default values
	user := model.User{
		Uuid:              uuid.NewString(),
		Name:              c.FormValue("name"),
		Username:          c.FormValue("username"),
		Email:             invitation.Email,
		SendToEmail:       "", // Not collected in invite form
		Password:          c.FormValue("password"),
		Role:              model.RoleRegular, // Invited users can only be regular users
		PreferredEpubType: "epub",            // Default to epub
		DefaultAction:     "download",
		WordsPerMinute:    u.config.WordsPerMinute,
	}

	// Validate user data
	errs := user.Validate(u.config.MinPasswordLength)
	errs = user.ConfirmPassword(c.FormValue("confirm-password"), u.config.MinPasswordLength, errs)

	// Check if username already exists
	existingUser, err := u.usersRepository.FindByUsername(user.Username)
	if err != nil {
		log.Printf("error checking for existing username: %v\n", err)
		return fiber.ErrInternalServerError
	}
	if existingUser != nil {
		errs["username"] = u.translator.T(lang, "This username is already taken")
	}

	if len(errs) > 0 {
		return c.Render("user/accept-invite", fiber.Map{
			"Title":            "Accept invitation",
			"InvitationUUID":   invitation.UUID,
			"Email":            invitation.Email,
			"Name":             user.Name,
			"Username":         user.Username,
			"Errors":           errs,
			"DisableLoginLink": true,
		}, "layout")
	}

	// Hash password
	user.Password = model.Hash(user.Password)

	// Create user
	if err := u.usersRepository.Create(&user); err != nil {
		log.Printf("error creating user: %v\n", err)
		return fiber.ErrInternalServerError
	}

	// Delete invitation
	if err := u.invitationsRepository.DeleteByEmail(invitation.Email); err != nil {
		log.Printf("error deleting invitation: %v\n", err)
	}

	c.Cookie(&fiber.Cookie{
		Name:    "success-once",
		Value:   u.translator.T(lang, "Account created successfully. Please log in."),
		Expires: time.Now().Add(24 * time.Hour),
	})

	return c.Redirect().To("/sessions/new")
}

func (u *Controller) validateInvitation(invitationUUID string) (*model.Invitation, error) {
	if _, ok := u.sender.(*infrastructure.NoEmail); ok {
		return nil, fiber.ErrNotFound
	}

	if invitationUUID == "" {
		return nil, fiber.ErrBadRequest
	}

	invitation, err := u.invitationsRepository.FindByUUID(invitationUUID)
	if err != nil {
		log.Printf("error finding invitation: %v\n", err)
		return nil, fiber.ErrInternalServerError
	}

	if invitation == nil {
		return nil, fiber.ErrNotFound
	}

	if invitation.ValidUntil.UTC().After(time.Now().UTC()) {
		return invitation, nil
	}

	// Invitation expired, delete it
	u.invitationsRepository.DeleteByEmail(invitation.Email)
	return nil, fiber.NewError(fiber.StatusBadRequest, "This invitation has expired")
}

func parseCommaSeparatedInviteEmails(raw string) []string {
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{})
	var out []string
	for _, p := range parts {
		e := strings.TrimSpace(p)
		if e == "" {
			continue
		}
		key := strings.ToLower(e)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, e)
	}
	return out
}

func (u *Controller) validateInviteCandidate(email, lang string) (string, error) {
	if _, err := mail.ParseAddress(email); err != nil {
		return u.translator.T(lang, "Incorrect email address: %s", email), nil
	}
	if len(email) > 100 {
		return u.translator.T(lang, "Email cannot be longer than 100 characters: %s", email), nil
	}
	existingUser, err := u.usersRepository.FindByEmail(email)
	if err != nil {
		log.Printf("error checking for existing user: %v\n", err)
		return "", err
	}
	if existingUser != nil {
		return u.translator.T(lang, "A user with this email already exists: %s", email), nil
	}
	return "", nil
}

func (u *Controller) validateAndPrepareInviteEmails(raw, lang string) (map[string]string, error) {
	errs := map[string]string{}

	maxList := u.config.InviteEmailListMaxLength
	maxRec := u.config.InviteMaxRecipients

	if len(raw) > maxList {
		errs["email"] = u.translator.T(lang, "Invitation list is too long (maximum %d characters)", maxList)
		return errs, nil
	}

	addresses := parseCommaSeparatedInviteEmails(raw)
	if len(addresses) == 0 {
		errs["email"] = u.translator.T(lang, "Enter at least one email address")
		return errs, nil
	}
	if len(addresses) > maxRec {
		errs["email"] = u.translator.T(lang, "Too many email addresses (maximum %d)", maxRec)
		return errs, nil
	}

	var problems []string
	for _, email := range addresses {
		msg, err := u.validateInviteCandidate(email, lang)
		if err != nil {
			return nil, err
		}
		if msg != "" {
			problems = append(problems, msg)
		}
	}
	if len(problems) > 0 {
		errs["email"] = strings.Join(problems, " ")
	}
	return errs, nil
}
