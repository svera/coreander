package webserver_test

import (
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/svera/coreander/internal/infrastructure"
	"github.com/svera/coreander/internal/model"
)

type SMTPMock struct {
	calledSend bool
	mu         sync.Mutex
	wg         sync.WaitGroup
}

func (s *SMTPMock) Send(address, subject, body string) error {
	defer s.wg.Done()

	s.mu.Lock()
	s.calledSend = true
	s.mu.Unlock()
	return nil
}

func (s *SMTPMock) SendDocument(address string, libraryPath string, fileName string) error {
	return nil
}

func TestAuthentication(t *testing.T) {
	db := infrastructure.Connect("file::memory:?cache=shared", 250)
	app := bootstrapApp(db, &infrastructure.SMTP{})

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
	app := bootstrapApp(db, &infrastructure.NoEmail{})

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
	db := infrastructure.Connect("file::memory:?cache=shared", 250)
	smtpMock := &SMTPMock{}
	app := bootstrapApp(db, smtpMock)

	data := url.Values{
		"email": {"admin@example.com"},
	}

	// Check that login page is accessible
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

	// Check that not posting an email returns an error
	req, err = http.NewRequest(http.MethodPost, "/en/recover", nil)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	response, err = app.Test(req)
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

	// Check that posting an existing email sends a recovery email
	req, err = http.NewRequest(http.MethodPost, "/en/recover", strings.NewReader(data.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	smtpMock.wg.Add(1)
	response, err = app.Test(req)
	smtpMock.wg.Wait()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	if expectedStatus := http.StatusOK; response.StatusCode != expectedStatus {
		t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
	}

	if !smtpMock.calledSend {
		t.Error("Email service 'send' method not called")
	}

	// Try to access the update password without the recovery ID from previous step
	req, err = http.NewRequest(http.MethodGet, "/en/reset-password", nil)
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

	// Try to access the reset password page with a recovery ID
	adminUser := model.User{}
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
	if expectedStatus := http.StatusOK; response.StatusCode != expectedStatus {
		t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
	}

	// Check that resetting the password successfully redirects to the login page
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
	req, err = http.NewRequest(http.MethodGet, "/en/reset-password", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	req.URL.RawQuery = q.Encode()

	response, err = app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if expectedStatus := http.StatusBadRequest; response.StatusCode != expectedStatus {
		t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
	}
}
