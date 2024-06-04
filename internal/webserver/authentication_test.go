package webserver_test

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"gorm.io/gorm"
)

func TestAuthentication(t *testing.T) {
	db := infrastructure.Connect("file::memory:", 250)
	app := bootstrapApp(db, &infrastructure.SMTP{}, afero.NewMemMapFs(), webserver.Config{})

	data := url.Values{
		"email":    {"admin@example.com"},
		"password": {"admin"},
	}

	t.Run("Try to log in with good and bad credentials", func(t *testing.T) {
		// Check that login page is accessible
		req, err := http.NewRequest(http.MethodGet, "/en/login", nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		if response.StatusCode != http.StatusOK {
			t.Errorf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
		}

		// Use no credentials to log in
		req, err = http.NewRequest(http.MethodPost, "/en/login", nil)
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		response, err = app.Test(req)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		if response.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status %d, received %d", http.StatusUnauthorized, response.StatusCode)
		}

		// Use good credentials to log in
		req, err = http.NewRequest(http.MethodPost, "/en/login", strings.NewReader(data.Encode()))
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		response, err = app.Test(req)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		if response.StatusCode != http.StatusFound {
			t.Errorf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
		}

		// Check that user is redirected to the home after a successful log in
		url, err := response.Location()
		if err != nil {
			t.Error("No location header present")
			return
		}
		if url.Path != "/en" {
			t.Errorf("Expected location %s, received %s", "/en", url.Path)
		}
	})
}

func TestRecoverNoEmailService(t *testing.T) {
	db := infrastructure.Connect("file::memory:?cache=shared", 250)
	app := bootstrapApp(db, &infrastructure.NoEmail{}, afero.NewMemMapFs(), webserver.Config{})

	req, err := http.NewRequest(http.MethodGet, "/en/recover", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if expectedStatus := http.StatusNotFound; response.StatusCode != expectedStatus {
		t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
	}
}

func TestRecover(t *testing.T) {
	var (
		db       *gorm.DB
		app      *fiber.App
		data     url.Values
		smtpMock *infrastructure.SMTPMock
	)

	reset := func(recoveryTimeout time.Duration) {
		t.Helper()

		webserverConfig := webserver.Config{
			SessionTimeout:        24 * time.Hour,
			RecoveryTimeout:       recoveryTimeout,
			LibraryPath:           "fixtures/library",
			UploadDocumentMaxSize: 1,
		}
		db = infrastructure.Connect("file::memory:?cache=shared", 250)
		smtpMock = &infrastructure.SMTPMock{}
		app = bootstrapApp(db, smtpMock, afero.NewMemMapFs(), webserverConfig)

		data = url.Values{
			"email": {"admin@example.com"},
		}
	}

	t.Run("Check that recover page is accessible", func(t *testing.T) {
		reset(2 * time.Hour)

		req, err := http.NewRequest(http.MethodGet, "/en/recover", nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		if expectedStatus := http.StatusOK; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
		}
	})

	t.Run("Check that not posting an email returns an error", func(t *testing.T) {
		reset(2 * time.Hour)

		req, err := http.NewRequest(http.MethodPost, "/en/recover", nil)
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		if expectedStatus := http.StatusOK; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
		}

		expectedErrorMessages := []string{
			"Incorrect email address",
		}

		doc, err := goquery.NewDocumentFromReader(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		errorMessages := []string{}
		doc.Find(".invalid-feedback").Each(func(i int, s *goquery.Selection) {
			errorMessages = append(errorMessages, strings.TrimSpace(s.Text()))
		})
		if !reflect.DeepEqual(expectedErrorMessages, errorMessages) {
			t.Errorf("Expected %v error messages, got %v", expectedErrorMessages, errorMessages)
		}
	})

	t.Run("Check that posting a non-existing email does not send an email", func(t *testing.T) {
		reset(2 * time.Hour)

		data = url.Values{
			"email": {"unknown@example.com"},
		}

		req, err := http.NewRequest(http.MethodPost, "/en/recover", strings.NewReader(data.Encode()))
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if expectedStatus := http.StatusOK; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
		}

		if smtpMock.CalledSend() {
			t.Error("Email service 'send' method called")
		}
	})

	t.Run("Try to access the update password without the recovery ID", func(t *testing.T) {
		reset(2 * time.Hour)

		req, err := http.NewRequest(http.MethodGet, "/en/reset-password", nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		if expectedStatus := http.StatusBadRequest; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
		}
	})

	t.Run("Check that posting an existing email sends a recovery email and resetting the password successfully redirects to the login page", func(t *testing.T) {
		reset(2 * time.Hour)

		req, err := http.NewRequest(http.MethodPost, "/en/recover", strings.NewReader(data.Encode()))
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		smtpMock.Wg.Add(1)
		response, err := app.Test(req)
		smtpMock.Wg.Wait()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if expectedStatus := http.StatusOK; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
		}

		if !smtpMock.CalledSend() {
			t.Error("Email service 'send' method not called")
		}

		// resetting the password successfully redirects to the login page
		adminUser := model.User{}
		db.Where("email = ?", "admin@example.com").First(&adminUser)

		data = url.Values{
			"password":         {"newPass"},
			"confirm-password": {"newPass"},
			"id":               {adminUser.RecoveryUUID},
		}

		req, err = http.NewRequest(http.MethodPost, "/en/reset-password", strings.NewReader(data.Encode()))
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		response, err = app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if expectedStatus := http.StatusFound; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
		}

		url, err := response.Location()
		if err != nil {
			t.Error("No location header present")
			return
		}
		if expectedURL := "/en/login"; url.Path != expectedURL {
			t.Errorf("Expected location %s, received %s", expectedURL, url.Path)
		}

		// Try to access again to the reset password page with the same recovery ID leads to an error
		db.Where("email = ?", "admin@example.com").First(&adminUser)

		req, err = http.NewRequest(http.MethodGet, "/en/reset-password", nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		q := req.URL.Query()
		q.Add("id", adminUser.RecoveryUUID)
		req.URL.RawQuery = q.Encode()

		response, err = app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		if expectedStatus := http.StatusBadRequest; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
		}
	})

	t.Run("Check that using a timed out link returns an error", func(t *testing.T) {
		reset(0 * time.Hour)

		req, err := http.NewRequest(http.MethodPost, "/en/recover", strings.NewReader(data.Encode()))
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		smtpMock.Wg.Add(1)
		response, err := app.Test(req)
		smtpMock.Wg.Wait()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if expectedStatus := http.StatusOK; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
		}

		// trying to access the reset password page using a time out ID returns an error
		adminUser := model.User{}
		db.Where("email = ?", "admin@example.com").First(&adminUser)

		req, err = http.NewRequest(http.MethodGet, fmt.Sprintf("/en/reset-password?id=%s", adminUser.RecoveryUUID), nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		response, err = app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if expectedStatus := http.StatusBadRequest; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
		}

		// trying to reset the password using a time out ID returns an error
		data = url.Values{
			"password":         {"newPass"},
			"confirm-password": {"newPass"},
			"id":               {adminUser.RecoveryUUID},
		}

		req, err = http.NewRequest(http.MethodPost, "/en/reset-password", strings.NewReader(data.Encode()))
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		response, err = app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if expectedStatus := http.StatusBadRequest; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
		}
	})
}
