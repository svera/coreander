package webserver_test

import (
	"net/http"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/metadata"
	"github.com/svera/coreander/internal/webserver"
)

func init() {
	_, filename, _, _ := runtime.Caller(0)
	os.Chdir(path.Dir(filename) + "/../..")
}

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
		"epub": metadata.NewMetadataReaderMock(),
	}
	app := webserver.New(index.NewReaderMock(), "", "", metadataReadersMock)

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
