package user

import (
	"github.com/svera/coreander/v4/internal/i18n"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

type Sender interface {
	From() string
}

type usersRepository interface {
	List(page int, resultsPerPage int) (result.Paginated[[]model.User], error)
	Total() int64
	FindByUuid(uuid string) (*model.User, error)
	FindByUsername(username string) (*model.User, error)
	Create(user *model.User) error
	Update(user *model.User) error
	FindByEmail(email string) (*model.User, error)
	Admins() int64
	Delete(uuid string) error
}

type Config struct {
	MinPasswordLength int
	WordsPerMinute    float64
	Secret            []byte
}

type Controller struct {
	repository usersRepository
	config     Config
	sender     Sender
	translator i18n.Translator
}

// NewController returns a new instance of the users controller
func NewController(repository usersRepository, usersCfg Config, sender Sender, translator i18n.Translator) *Controller {
	return &Controller{
		repository: repository,
		config:     usersCfg,
		sender:     sender,
		translator: translator,
	}
}
