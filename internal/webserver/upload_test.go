package webserver_test

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
	"github.com/svera/coreander/v3/internal/webserver/model"
)

func TestUpload(t *testing.T) {
	db := infrastructure.Connect("file::memory:", 250)
	app := bootstrapApp(db, &infrastructure.NoEmail{}, afero.NewMemMapFs())

	data := url.Values{
		"name":             {"Test user"},
		"email":            {"test@example.com"},
		"password":         {"test"},
		"confirm-password": {"test"},
		"role":             {fmt.Sprint(model.RoleRegular)},
		"words-per-minute": {"250"},
	}

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

	t.Run("Try to access upload page with a regular user session", func(t *testing.T) {
		response, err := postRequest(data, adminCookie, app, "/en/users/new")
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		cookie, err := login(app, "test@example.com", "test")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		response, err = getRequest(cookie, app, "/en/upload")
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
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
		var buf bytes.Buffer
		multipartWriter := multipart.NewWriter(&buf)

		// add form field
		filePart, _ := multipartWriter.CreateFormFile("filename", "file.txt")
		filePart.Write([]byte("Hello, World!"))

		multipartWriter.Close()
		req, err := http.NewRequest(http.MethodPost, "/en/upload", &buf)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		req.Header.Add("Content-Type", multipartWriter.FormDataContentType())
		req.AddCookie(adminCookie)

		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if expectedStatus := http.StatusBadRequest; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, got %d", expectedStatus, response.StatusCode)
		}
	})

	t.Run("Returns 200 for file content-type allowed", func(t *testing.T) {
		var buf bytes.Buffer
		multipartWriter := multipart.NewWriter(&buf)

		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, "filename", "file.txt"))
		h.Set("Content-Type", "application/epub+zip")
		part, _ := multipartWriter.CreatePart(h)
		part.Write([]byte(`sample`))
		multipartWriter.Close()

		req, err := http.NewRequest(http.MethodPost, "/en/upload", &buf)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		req.Header.Add("Content-Type", multipartWriter.FormDataContentType())
		req.AddCookie(adminCookie)

		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if expectedStatus := http.StatusFound; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, got %d", expectedStatus, response.StatusCode)
		}
	})

	t.Run("Returns 400 when trying to send no file", func(t *testing.T) {
		var buf bytes.Buffer
		multipartWriter := multipart.NewWriter(&buf)
		multipartWriter.Close()

		req, err := http.NewRequest(http.MethodPost, "/en/upload", &buf)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		req.Header.Add("Content-Type", multipartWriter.FormDataContentType())
		req.AddCookie(adminCookie)

		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if expectedStatus := http.StatusBadRequest; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, got %d", expectedStatus, response.StatusCode)
		}
	})
}
