package user

import (
	"fmt"
	"log"
	"net/mail"
	"strconv"
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
	errs := map[string]string{}

	// Validate email
	if _, err := mail.ParseAddress(email); err != nil {
		errs["email"] = "Incorrect email address"
	}

	if len(email) > 100 {
		errs["email"] = "Email cannot be longer than 100 characters"
	}

	// Check if user already exists
	existingUser, err := u.repository.FindByEmail(email)
	if err != nil {
		log.Printf("error checking for existing user: %v\n", err)
		return fiber.ErrInternalServerError
	}
	if existingUser != nil {
		errs["email"] = "A user with this email already exists"
	}

	// Check if there's already a pending invitation
	existingInvitation, err := u.invitationsRepository.FindByEmail(email)
	if err != nil {
		log.Printf("error checking for existing invitation: %v\n", err)
		return fiber.ErrInternalServerError
	}
	if existingInvitation != nil {
		// Delete old invitation before creating a new one
		if err := u.invitationsRepository.DeleteByEmail(email); err != nil {
			log.Printf("error deleting old invitation: %v\n", err)
			return fiber.ErrInternalServerError
		}
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
	invitationLink := fmt.Sprintf(
		"%s/users/accept-invite?id=%s",
		u.config.FQDN,
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
		Value:   fmt.Sprintf("Invitation sent successfully to %s", email),
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
		"Title":          "Accept invitation",
		"InvitationUUID": invitation.UUID,
		"Email":          invitation.Email,
		"Errors":         map[string]string{},
	}, "layout")
}

// AcceptInvite processes the invitation acceptance
func (u *Controller) AcceptInvite(c *fiber.Ctx) error {
	invitation, err := u.validateInvitation(c.FormValue("invitation_uuid"))
	if err != nil {
		return err
	}

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
	existingUser, err := u.repository.FindByUsername(user.Username)
	if err != nil {
		log.Printf("error checking for existing username: %v\n", err)
		return fiber.ErrInternalServerError
	}
	if existingUser != nil {
		errs["username"] = "This username is already taken"
	}

	if len(errs) > 0 {
		return c.Render("user/accept-invite", fiber.Map{
			"Title":          "Accept invitation",
			"InvitationUUID": invitation.UUID,
			"Email":          invitation.Email,
			"Name":           user.Name,
			"Username":       user.Username,
			"Errors":         errs,
		}, "layout")
	}

	// Hash password
	user.Password = model.Hash(user.Password)

	// Create user
	if err := u.repository.Create(&user); err != nil {
		log.Printf("error creating user: %v\n", err)
		return fiber.ErrInternalServerError
	}

	// Delete invitation
	if err := u.invitationsRepository.Delete(invitation.UUID); err != nil {
		log.Printf("error deleting invitation: %v\n", err)
	}

	c.Cookie(&fiber.Cookie{
		Name:    "success-once",
		Value:   "Account created successfully. Please log in.",
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
	u.invitationsRepository.Delete(invitationUUID)
	return nil, fiber.NewError(fiber.StatusBadRequest, "This invitation has expired")
}
