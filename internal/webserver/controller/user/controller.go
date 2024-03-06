package user

import (
	"github.com/svera/coreander/v3/internal/result"
	"github.com/svera/coreander/v3/internal/webserver/model"
)

type usersRepository interface {
	List(page int, resultsPerPage int) (result.Paginated[[]model.User], error)
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
