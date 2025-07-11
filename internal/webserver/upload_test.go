package webserver_test

import (
	"bytes"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func TestUpload(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	appFS := loadDirInMemoryFs("fixtures/library")
	app := bootstrapApp(db, &infrastructure.NoEmail{}, appFS, webserver.Config{})

	data := url.Values{
		"name":             {"Test user"},
		"username":         {"test"},
		"email":            {"test@example.com"},
		"password":         {"test"},
		"confirm-password": {"test"},
		"role":             {fmt.Sprint(model.RoleRegular)},
		"words-per-minute": {"250"},
	}

	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	response, err := postRequest(data, adminCookie, app, "/users", t)
	if response == nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	regularUserCookie, err := login(app, "test@example.com", "test", t)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	t.Run("Try to access upload page without an active session", func(t *testing.T) {
		response, err := getRequest(&http.Cookie{}, app, "/upload", t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnForbiddenAndShowLogin(response, t)
	})

	t.Run("Try to access upload page with a regular user session", func(t *testing.T) {
		response, err = getRequest(regularUserCookie, app, "/upload", t)
		if response == nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
	})

	t.Run("Try to upload a document with a regular user session", func(t *testing.T) {
		var buf bytes.Buffer
		multipartWriter := multipart.NewWriter(&buf)
		multipartWriter.Close()

		req, err := http.NewRequest(http.MethodPost, "/documents", &buf)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		req.Header.Set("Accept-Language", "en")
		req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		req.AddCookie(regularUserCookie)

		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
	})

	t.Run("Try to access upload page with an admin active session", func(t *testing.T) {
		response, err := getRequest(adminCookie, app, "/upload", t)
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
		req, err := http.NewRequest(http.MethodPost, "/documents", &buf)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		req.Header.Set("Accept-Language", "en")
		req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		req.AddCookie(adminCookie)

		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if expectedStatus := http.StatusBadRequest; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, got %d", expectedStatus, response.StatusCode)
		}
	})

	t.Run("Returns 500 if a document was uploaded correctly but couldn't be indexed", func(t *testing.T) {
		var buf bytes.Buffer
		multipartWriter := multipart.NewWriter(&buf)

		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, "filename", "file.txt"))
		h.Set("Content-Type", "application/epub+zip")
		part, _ := multipartWriter.CreatePart(h)
		part.Write([]byte(`sample`))
		multipartWriter.Close()

		req, err := http.NewRequest(http.MethodPost, "/documents", &buf)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		req.Header.Set("Accept-Language", "en")
		req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		req.AddCookie(adminCookie)

		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if expectedStatus := http.StatusInternalServerError; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, got %d", expectedStatus, response.StatusCode)
		}
	})

	t.Run("Returns 400 when trying to send no file", func(t *testing.T) {
		var buf bytes.Buffer
		multipartWriter := multipart.NewWriter(&buf)
		multipartWriter.Close()

		req, err := http.NewRequest(http.MethodPost, "/documents", &buf)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		req.Header.Set("Accept-Language", "en")
		req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		req.AddCookie(adminCookie)

		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if expectedStatus := http.StatusBadRequest; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, got %d", expectedStatus, response.StatusCode)
		}
	})

	t.Run("Returns 413 for file too big", func(t *testing.T) {
		var buf bytes.Buffer
		multipartWriter := multipart.NewWriter(&buf)

		file, err := os.ReadFile("fixtures/upload/haruko-html-jpeg.epub")
		if err != nil {
			log.Fatal(err)
		}

		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, "filename", "haruko-html-jpeg.epub"))
		h.Set("Content-Type", "application/epub+zip")
		part, _ := multipartWriter.CreatePart(h)
		part.Write(file)

		multipartWriter.Close()

		req, err := http.NewRequest(http.MethodPost, "/documents", &buf)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		req.Header.Set("Accept-Language", "en")
		req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		req.AddCookie(adminCookie)

		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if expectedStatus := http.StatusRequestEntityTooLarge; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, got %d", expectedStatus, response.StatusCode)
		}
	})

	// Due to a limitation in how pirmd/epub handles opening epub files, we need to use
	// a real filesystem instead Afero's in-memory implementation
	t.Run("Returns 302 for correct document", func(t *testing.T) {
		fs := afero.NewOsFs()
		app := bootstrapApp(db, &infrastructure.NoEmail{}, fs, webserver.Config{})

		t.Cleanup(func() {
			fs.Remove("fixtures/library/childrens-literature.epub")
		})

		var buf bytes.Buffer
		multipartWriter := multipart.NewWriter(&buf)

		file, err := os.ReadFile("fixtures/upload/childrens-literature.epub")
		if err != nil {
			log.Fatal(err)
		}

		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, "filename", "childrens-literature.epub"))
		h.Set("Content-Type", "application/epub+zip")
		part, _ := multipartWriter.CreatePart(h)
		part.Write(file)

		multipartWriter.Close()

		req, err := http.NewRequest(http.MethodPost, "/documents", &buf)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		req.Header.Set("Accept-Language", "en")
		req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		req.AddCookie(adminCookie)

		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if expectedStatus := http.StatusFound; response.StatusCode != expectedStatus {
			t.Errorf("Expected status %d, got %d", expectedStatus, response.StatusCode)
		}

		// The recently added document should appear in home page under "Latest additions"
		req, err = http.NewRequest(http.MethodGet, "/", &buf)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		req.Header.Set("Accept-Language", "en")
		response, err = app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		doc, err := goquery.NewDocumentFromReader(response.Body)
		if err != nil {
			t.Fatal(err)
		}

		if expectedResults, actualResults := 1, doc.Find("h2:contains(\"Latest additions\")").Length(); actualResults != expectedResults {
			t.Errorf("Expected %d results, got %d", expectedResults, actualResults)
		}
	})
}
