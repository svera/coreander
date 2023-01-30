package webserver_test

import (
	"net/http"
	"testing"

	"github.com/svera/coreander/internal/infrastructure"
	"github.com/svera/coreander/internal/metadata"
	"github.com/svera/coreander/internal/webserver"
)

func TestGET(t *testing.T) {
	var cases = []struct {
		name           string
		url            string
		expectedStatus int
	}{
		{"Redirect if the user tries to access to the root URL", "/", http.StatusFound},
		{"Page loads succesfully if the user tries to access an existent URL", "/es", http.StatusOK},
		{"Server returns not found if the user tries to access a non-existent URL", "/xx", http.StatusNotFound},
	}

	metadataReadersMock := map[string]metadata.Reader{
		"epub": metadata.NewReaderMock(),
	}

	db := infrastructure.Connect("file::memory:?cache=shared")
	app := webserver.New(webserver.NewReaderMock(), "", "", "", metadataReadersMock, 300, &infrastructure.NoEmail{}, db)

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tcase.url, nil)

			body, err := app.Test(req)
			if err != nil {
				t.Errorf("Unexpected error: %v", err.Error())
			}
			if body.StatusCode != tcase.expectedStatus {
				t.Errorf("Wrong status code received, expected %d, got %d", tcase.expectedStatus, body.StatusCode)
			}
		})
	}
}
