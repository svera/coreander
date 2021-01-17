package webserver_test

import (
	"net/http"
	"testing"

	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/webserver"
)

func TestGET(t *testing.T) {
	app := webserver.New(index.NewReaderMock(), "")
	req, _ := http.NewRequest("GET", "", nil)

	body, err := app.Test(req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err.Error())
	}
	if body.StatusCode != http.StatusOK {
		t.Errorf("Wrong status code received, expected %d, got %d", http.StatusOK, body.StatusCode)
	}
}
