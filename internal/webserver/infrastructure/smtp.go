package infrastructure

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"gopkg.in/gomail.v2"
)

type SMTP struct {
	Server   string
	Port     int
	User     string
	Password string
}

func (s *SMTP) Send(address, subject, body string) error {
	m := s.compose(address, subject, body)

	return s.send(m)
}

// SendDocument sends an email with the designated file attached to it to the chosen address
func (s *SMTP) SendDocument(address string, libraryPath string, fileName string) error {
	m := s.compose(address, "", "")
	m.Attach(fmt.Sprintf("%s/%s", libraryPath, fileName))

	return s.send(m)
}

func (s *SMTP) From() string {
	return s.User
}

func (s *SMTP) compose(address, subject, body string) *gomail.Message {
	m := gomail.NewMessage()
	m.SetHeader("From", fmt.Sprintf("%s <%s>", "Coreander", s.User))
	m.SetHeader("To", address)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", body)

	return m
}

func (s *SMTP) send(m *gomail.Message) error {
	d := gomail.NewDialer(s.Server, s.Port, s.User, s.Password)

	if err := d.DialAndSend(m); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return nil
}
