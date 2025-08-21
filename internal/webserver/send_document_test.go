package webserver_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
)

func TestSendDocument(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	app := bootstrapApp(db, smtpMock, afero.NewOsFs(), webserver.Config{})

	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	var cases = []struct {
		name               string
		cookie             *http.Cookie
		email              string
		slug               string
		expectedHTTPStatus int
	}{
		{"Send no email address", adminCookie, "", "empty", http.StatusBadRequest},
		{"Send non existing document slug", adminCookie, "admin@example.com", "wrong", http.StatusNotFound},
		{"Send document slug and email address while being unauthenticated", nil, "admin@example.com", "john-doe-test-epub", http.StatusForbidden},
		{"Send document slug and email address", adminCookie, "admin@example.com", "john-doe-test-epub", http.StatusOK},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			var (
				response *http.Response
				err      error
			)

			data := url.Values{
				"email": {tcase.email},
			}

			req, err := http.NewRequest(http.MethodPost, "/documents/"+tcase.slug+"/send", strings.NewReader(data.Encode()))
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}

			req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
			if tcase.cookie != nil {
				req.AddCookie(tcase.cookie)
			}

			if tcase.expectedHTTPStatus == http.StatusOK {
				smtpMock.Wg.Add(1)
				response, err = app.Test(req)
				smtpMock.Wg.Wait()
			} else {
				response, err = app.Test(req)
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}

			if response.StatusCode != tcase.expectedHTTPStatus {
				t.Errorf("Expected status %d, received %d", tcase.expectedHTTPStatus, response.StatusCode)
			}
		})
	}
}
