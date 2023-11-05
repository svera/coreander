package auth

import "github.com/gofiber/fiber/v2"

func (a *Controller) EditPassword(c *fiber.Ctx) error {
	if _, err := a.validateRecoveryAccess(c, c.Query("id")); err != nil {
		return err
	}

	return c.Render("auth/edit-password", fiber.Map{
		"Title":  "Reset password",
		"Uuid":   c.Query("id"),
		"Errors": map[string]string{},
	}, "layout")
}
