package auth

import (
	"time"

	"github.com/svera/coreander/v3/internal/webserver/model"
	"golang.org/x/text/message"
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
	printers   map[string]*message.Printer
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

func NewController(repository authRepository, sender recoveryEmail, cfg Config, printers map[string]*message.Printer) *Controller {
	return &Controller{
		repository: repository,
		sender:     sender,
		printers:   printers,
		config:     cfg,
	}
}
