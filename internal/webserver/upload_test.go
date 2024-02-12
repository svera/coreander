package webserver_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"testing"

	"github.com/spf13/afero"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
)

func TestUpload(t *testing.T) {
	db := infrastructure.Connect("file::memory:", 250)
	app := bootstrapApp(db, &infrastructure.NoEmail{}, afero.NewMemMapFs())

	adminCookie, err := login(app, "admin@example.com", "admin")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	t.Run("Try to access upload page without an active session", func(t *testing.T) {
		response, err := getRequest(&http.Cookie{}, app, "/en/upload")
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)
	})

	t.Run("Try to access upload page with an admin active session", func(t *testing.T) {
		response, err := getRequest(adminCookie, app, "/en/upload")
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		if expectedStatus := http.StatusOK; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, got %d", expectedStatus, response.StatusCode)
		}
	})

	t.Run("Returns 400 for file content-type not allowed", func(t *testing.T) {
		jsonD, err := json.Marshal(struct{ TestField string }{TestField: "test"})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		body := new(bytes.Buffer)
		writer := multipart.NewWriter(body)
		writer.WriteField("payload", string(jsonD))
		// Create the file part with an unsupported content-type
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, EscapeQuotes("filename"), EscapeQuotes("file.txt")))
		h.Set("Content-Type", "application/octet-stream")
		part, _ := writer.CreatePart(h)
		part.Write([]byte(`sample`))
		writer.Close()
	})
}
