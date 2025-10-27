package user

import (
	"time"

	"github.com/svera/coreander/v4/internal/i18n"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

type Sender interface {
	From() string
	Send(address, subject, body string) error
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

type invitationsRepository interface {
	Create(invitation *model.Invitation) error
	FindByUUID(uuid string) (*model.Invitation, error)
	DeleteByEmail(email string) error
}

type Config struct {
	MinPasswordLength int
	WordsPerMinute    float64
	Secret            []byte
	InvitationTimeout time.Duration
	FQDN              string
}

type Controller struct {
	usersRepository       usersRepository
	invitationsRepository invitationsRepository
	config                Config
	sender                Sender
	translator            i18n.Translator
}

// NewController returns a new instance of the users controller
func NewController(repository usersRepository, invitationsRepository invitationsRepository, usersCfg Config, sender Sender, translator i18n.Translator) *Controller {
	return &Controller{
		usersRepository:       repository,
		invitationsRepository: invitationsRepository,
		config:                usersCfg,
		sender:                sender,
		translator:            translator,
	}
}
