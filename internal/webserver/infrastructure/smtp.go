package infrastructure

import (
	"bytes"
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/wneessen/go-mail"
)

type SMTP struct {
	Server   string
	Port     int
	User     string
	Password string
}

func (s *SMTP) client() (*mail.Client, error) {
	return mail.NewClient(s.Server,
		mail.WithPort(s.Port),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(s.User),
		mail.WithPassword(s.Password),
	)
}

func (s *SMTP) Send(address, subject, body string) error {
	client, err := s.client()
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	m := mail.NewMsg()
	m.FromFormat("Coreander", s.User)
	if err := m.To(address); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	m.Subject(subject)
	m.SetBodyString(mail.TypeTextHTML, body)
	if err := client.DialAndSend(m); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}

func (s *SMTP) SendBCC(addresses []string, subject, body string) error {
	if len(addresses) == 0 {
		return nil
	}
	client, err := s.client()
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	m := mail.NewMsg()
	m.FromFormat("Coreander", s.User)
	if err := m.Bcc(addresses...); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	m.Subject(subject)
	m.SetBodyString(mail.TypeTextHTML, body)
	if err := client.DialAndSend(m); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}

// SendDocument sends an email with the given file attached to the chosen address.
func (s *SMTP) SendDocument(address, subject string, file []byte, fileName string) error {
	client, err := s.client()
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	m := mail.NewMsg()
	m.FromFormat("Coreander", s.User)
	if err := m.To(address); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	m.Subject(subject)
	m.SetBodyString(mail.TypeTextHTML, "")
	if err := m.AttachReader(fileName, bytes.NewReader(file)); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	if err := client.DialAndSend(m); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}

func (s *SMTP) From() string {
	return s.User
}
