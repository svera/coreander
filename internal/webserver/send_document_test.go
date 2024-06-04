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
	db := infrastructure.Connect("file::memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	app := bootstrapApp(db, smtpMock, afero.NewOsFs(), webserver.Config{})

	var cases = []struct {
		name               string
		email              string
		slug               string
		expectedHTTPStatus int
	}{
		{"Send no document slug", "admin@example.com", "", http.StatusBadRequest},
		{"Send no email address", "", "empty", http.StatusBadRequest},
		{"Send non existing document slug", "admin@example.com", "wrong", http.StatusBadRequest},
		{"Send document slug and email address", "admin@example.com", "john-doe-test-epub", http.StatusOK},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			var (
				response *http.Response
				err      error
			)

			data := url.Values{
				"email": {tcase.email},
				"slug":  {tcase.slug},
			}

			req, err := http.NewRequest(http.MethodPost, "/send", strings.NewReader(data.Encode()))
			req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
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
