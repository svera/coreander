package user

import (
	"fmt"
	"log"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// InviteForm shows the invitation form
func (u *Controller) InviteForm(c *fiber.Ctx) error {
	if _, ok := u.sender.(*infrastructure.NoEmail); ok {
		return fiber.ErrNotFound
	}

	return c.Render("user/invite-form", fiber.Map{
		"Title":  "Invite user",
		"Errors": map[string]string{},
	}, "layout")
}

// SendInvite sends an invitation email to the specified address
func (u *Controller) SendInvite(c *fiber.Ctx) error {
	if _, ok := u.sender.(*infrastructure.NoEmail); ok {
		return fiber.ErrNotFound
	}

	email := c.FormValue("email")
	lang := c.Locals("Lang").(string)

	errs, err := u.validateInviteEmail(email, lang)
	if err != nil {
		return err
	}

	if len(errs) > 0 {
		return c.Render("user/invite-form", fiber.Map{
			"Title":  "Invite user",
			"Email":  email,
			"Errors": errs,
		}, "layout")
	}

	// Create invitation
	invitation := &model.Invitation{
		Email:      email,
		UUID:       uuid.NewString(),
		ValidUntil: time.Now().UTC().Add(u.config.InvitationTimeout),
	}

	if err := u.invitationsRepository.Create(invitation); err != nil {
		log.Printf("error creating invitation: %v\n", err)
		return fiber.ErrInternalServerError
	}

	// Send invitation email
	fqdn := u.config.FQDN
	// Ensure FQDN has a protocol
	if !strings.HasPrefix(fqdn, "http://") && !strings.HasPrefix(fqdn, "https://") {
		fqdn = "http://" + fqdn
	}

	invitationLink := fmt.Sprintf(
		"%s/invite?id=%s",
		fqdn,
		invitation.UUID,
	)

	c.Render("user/invitation-email", fiber.Map{
		"InvitationLink":    invitationLink,
		"InvitationTimeout": strconv.FormatFloat(u.config.InvitationTimeout.Hours(), 'f', -1, 64),
	})

	if err := u.sender.Send(
		email,
		u.translator.T(c.Locals("Lang").(string), "You've been invited to join Coreander"),
		string(c.Response().Body()),
	); err != nil {
		log.Printf("error sending invitation email: %v\n", err)
		return fiber.ErrInternalServerError
	}

	c.Cookie(&fiber.Cookie{
		Name:    "success-once",
		Value:   u.translator.T(lang, "Invitation sent successfully to %s", email),
		Expires: time.Now().Add(24 * time.Hour),
	})

	return c.Redirect("/users")
}

// AcceptInviteForm shows the form to accept an invitation
func (u *Controller) AcceptInviteForm(c *fiber.Ctx) error {
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
func (u *Controller) AcceptInvite(c *fiber.Ctx) error {
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

	return c.Redirect("/sessions/new")
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

func (u *Controller) validateInviteEmail(email, lang string) (map[string]string, error) {
	errs := map[string]string{}

	// Validate email format
	if _, err := mail.ParseAddress(email); err != nil {
		errs["email"] = u.translator.T(lang, "Incorrect email address")
	}

	// Validate email length
	if len(email) > 100 {
		errs["email"] = u.translator.T(lang, "Email cannot be longer than 100 characters")
	}

	// Check if user already exists
	existingUser, err := u.usersRepository.FindByEmail(email)
	if err != nil {
		log.Printf("error checking for existing user: %v\n", err)
		return nil, fiber.ErrInternalServerError
	}
	if existingUser != nil {
		errs["email"] = u.translator.T(lang, "A user with this email already exists")
	}

	// Delete any existing invitation for this email before creating a new one
	if err := u.invitationsRepository.DeleteByEmail(email); err != nil {
		log.Printf("error deleting old invitation: %v\n", err)
		return nil, fiber.ErrInternalServerError
	}

	return errs, nil
}
