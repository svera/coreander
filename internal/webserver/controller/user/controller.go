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

type readingRepository interface {
	CompletedBetweenDates(userID int, startDate, endDate *time.Time) ([]string, error)
}

type indexer interface {
	TotalWordCount(IDs []string) (float64, error)
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
	readingRepository     readingRepository
	indexer               indexer
	config                Config
	sender                Sender
	translator            i18n.Translator
}

// NewController returns a new instance of the users controller
func NewController(usersRepository usersRepository, invitationsRepository invitationsRepository, readingRepository readingRepository, indexer indexer, usersCfg Config, sender Sender, translator i18n.Translator) *Controller {
	return &Controller{
		usersRepository:       usersRepository,
		invitationsRepository: invitationsRepository,
		readingRepository:     readingRepository,
		indexer:               indexer,
		config:                usersCfg,
		sender:                sender,
		translator:            translator,
	}
}
