package webserver_test

import (
	"net/http"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/webserver"
)

func init() {
	_, filename, _, _ := runtime.Caller(0)
	os.Chdir(path.Dir(filename) + "/../..")
}

func TestGET(t *testing.T) {
	var cases = []struct {
		url            string
		expectedStatus int
	}{
		{"/", http.StatusMovedPermanently},
		{"/es", http.StatusOK},
		{"/xx", http.StatusNotFound},
	}
	app := webserver.New(index.NewReaderMock(), "")

	for _, tt := range cases {
		t.Run(tt.url, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.url, nil)

			body, err := app.Test(req)
			if err != nil {
				t.Errorf("Unexpected error: %v", err.Error())
			}
			if body.StatusCode != tt.expectedStatus {
				t.Errorf("Wrong status code received, expected %d, got %d", tt.expectedStatus, body.StatusCode)
			}
		})
	}
}
