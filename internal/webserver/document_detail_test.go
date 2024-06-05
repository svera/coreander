package webserver_test

import (
	"net/http"
	"testing"

	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
)

func TestDocumentAndRead(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	app := bootstrapApp(db, smtpMock, afero.NewOsFs(), webserver.Config{})

	var cases = []struct {
		url            string
		expectedStatus int
	}{
		{"/en/read/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha", http.StatusOK},
		{"/en/read/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha-2", http.StatusOK},
		{"/en/document/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha", http.StatusOK},
	}

	for _, tcase := range cases {
		t.Run(tcase.url, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tcase.url, nil)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
			response, err := app.Test(req)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
			if response.StatusCode != tcase.expectedStatus {
				t.Errorf("Expected status %d, received %d", tcase.expectedStatus, response.StatusCode)
			}
		})
	}
}
