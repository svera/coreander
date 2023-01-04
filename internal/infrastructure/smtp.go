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

func (s *SMTP) SendDocument(address string, libraryPath string, fileName string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", s.User)
	m.SetHeader("To", address)
	// Both subject and Body must be set, even being emtpy, for the document to be delivered to Kindle devices
	m.SetHeader("Subject", "")
	m.SetBody("text/html", "")
	m.Attach(fmt.Sprintf("%s/%s", libraryPath, fileName))

	d := gomail.NewDialer(s.Server, s.Port, s.User, s.Password)

	if err := d.DialAndSend(m); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return nil
}
