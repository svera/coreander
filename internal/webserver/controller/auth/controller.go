package auth

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/model"
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
}

const (
	defaultHttpPort  = 80
	defaultHttpsPort = 443
)

func NewController(repository authRepository, sender recoveryEmail, cfg Config, printers map[string]*message.Printer) *Controller {
	return &Controller{
		repository: repository,
		sender:     sender,
		printers:   printers,
		config:     cfg,
	}
}

func (a *Controller) urlPort(c *fiber.Ctx) string {
	port := fmt.Sprintf(":%d", a.config.Port)
	if (a.config.Port == defaultHttpPort && c.Protocol() == "http") ||
		(a.config.Port == defaultHttpsPort && c.Protocol() == "https") {
		port = ""
	}
	return port
}
