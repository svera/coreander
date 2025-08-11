package auth

import (
	"time"

	"github.com/svera/coreander/v4/internal/i18n"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

type authRepository interface {
	FindByEmail(email string) (*model.User, error)
	FindByRecoveryUuid(recoveryUuid string) (*model.User, error)
	Update(user *model.User) error
}

type recoveryEmail interface {
	Send(address, subject, body string) error
}

type Controller struct {
	repository authRepository
	sender     recoveryEmail
	printers   *i18n.Printers
	config     Config
}

type Config struct {
	Secret            []byte
	MinPasswordLength int
	Hostname          string
	Port              int
	SessionTimeout    time.Duration
	RecoveryTimeout   time.Duration
}

func NewController(repository authRepository, sender recoveryEmail, cfg Config, printers *i18n.Printers) *Controller {
	return &Controller{
		repository: repository,
		sender:     sender,
		printers:   printers,
		config:     cfg,
	}
}
