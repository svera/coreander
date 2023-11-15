package user

import (
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/model"
	"github.com/svera/coreander/v3/internal/search"
	"github.com/svera/coreander/v3/internal/webserver/jwtclaimsreader"
)

type usersRepository interface {
	List(page int, resultsPerPage int) (search.PaginatedResult[[]model.User], error)
	Total() int64
	FindByUuid(uuid string) (*model.User, error)
	Create(user *model.User) error
	Update(user *model.User) error
	FindByEmail(email string) (*model.User, error)
	Admins() int64
	Delete(uuid string) error
}

type Config struct {
	MinPasswordLength int
	WordsPerMinute    float64
}

type Controller struct {
	repository usersRepository
	config     Config
}

// NewController returns a new instance of the users controller
func NewController(repository usersRepository, usersCfg Config) *Controller {
	return &Controller{
		repository: repository,
		config:     usersCfg,
	}
}

// New renders the new user form
func (u *Controller) New(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin {
		return fiber.ErrForbidden
	}

	user := model.User{
		WordsPerMinute: u.config.WordsPerMinute,
	}
	return c.Render("users/new", fiber.Map{
		"Title":             "Add user",
		"Session":           session,
		"MinPasswordLength": u.config.MinPasswordLength,
		"User":              user,
		"Errors":            map[string]string{},
	}, "layout")
}
