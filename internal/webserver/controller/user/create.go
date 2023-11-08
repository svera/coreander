package user

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/svera/coreander/v3/internal/model"
	"github.com/svera/coreander/v3/internal/webserver/jwtclaimsreader"
)

// Create gathers information coming from the new user form and creates a new user
func (u *Controller) Create(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin {
		return fiber.ErrForbidden
	}

	role, _ := strconv.Atoi(c.FormValue("role"))
	user := model.User{
		Name:     c.FormValue("name"),
		Email:    c.FormValue("email"),
		Password: c.FormValue("password"),
		Role:     role,
		Uuid:     uuid.NewString(),
	}
	user.WordsPerMinute, _ = strconv.ParseFloat(c.FormValue("words-per-minute"), 64)

	errs := user.Validate(u.config.MinPasswordLength)
	if exist, _ := u.repository.FindByEmail(c.FormValue("email")); exist != nil {
		errs["email"] = "A user with this email address already exist"
	}

	if errs = user.ConfirmPassword(c.FormValue("confirm-password"), u.config.MinPasswordLength, errs); len(errs) > 0 {
		return c.Render("users/new", fiber.Map{
			"Title":   "Add user",
			"Session": session,
			"Errors":  errs,
			"User":    user,
		}, "layout")
	}

	user.Password = model.Hash(user.Password)
	if err := u.repository.Create(&user); err != nil {
		return fiber.ErrInternalServerError
	}

	return c.Redirect(fmt.Sprintf("/%s/users", c.Params("lang")))
}
