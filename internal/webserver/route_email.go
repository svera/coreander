package webserver

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	gomail "gopkg.in/gomail.v2"
)

func routeSend(c *fiber.Ctx, libraryPath string, fileName string, address string, smtpSettings SMTP) error {
	err := send(address, libraryPath, smtpSettings, fileName)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return nil
}

func send(address string, libraryPath string, smtpSettings SMTP, fileName string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", smtpSettings.User)
	m.SetHeader("To", address)
	m.SetHeader("Subject", "Hello!")
	m.SetBody("text/html", "Hello <b>Bob</b> and <i>Cora</i>!")
	m.Attach(fmt.Sprintf("%s/%s", libraryPath, fileName))

	d := gomail.NewDialer(smtpSettings.Server, smtpSettings.Port, smtpSettings.User, smtpSettings.Password)

	if err := d.DialAndSend(m); err != nil {
		return err
	}
	return nil
}
