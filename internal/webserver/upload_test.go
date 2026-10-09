package webserver_test

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
	"github.com/svera/coreander/v5/internal/result"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/controller/document"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

type busyUploadIndex struct {
	document.IdxReaderWriter
}

func (busyUploadIndex) NewFile(string, []byte) (string, error) {
	return "", index.ErrLibraryIndexing
}

func (busyUploadIndex) DeleteDocument(string) error {
	return index.ErrLibraryIndexing
}

type uploadErrorView struct{}

func (uploadErrorView) Load() error { return nil }

func (uploadErrorView) Render(out io.Writer, _ string, binding any, _ ...string) error {
	_, err := fmt.Fprint(out, binding.(fiber.Map)["Error"])
	return err
}

func TestUploadRejectedDuringBulkIndexing(t *testing.T) {
	controller := document.NewController(nil, nil, nil, nil, nil, busyUploadIndex{}, nil, document.Config{}, nil)
	app := fiber.New(fiber.Config{Views: uploadErrorView{}})
	app.Post("/documents", controller.Upload)
	app.Delete("/documents/:slug", controller.Delete)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="filename"; filename="book.epub"`)
	header.Set("Content-Type", "application/epub+zip")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("book")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, "/documents", &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.StatusCode)
	}
	message, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(message) != "Library indexing is running. Please try uploading again later." {
		t.Fatalf("unexpected error message: %q", message)
	}
	request, err = http.NewRequest(http.MethodDelete, "/documents/book", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err = app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("deletion status = %d, want 503", response.StatusCode)
	}
	message, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(message) != "Library indexing is running. Please try deleting again later." {
		t.Fatalf("unexpected deletion error message: %q", message)
	}
}

func TestDocumentDeleteButtonDuringIndexing(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	app := bootstrapApp(db, &infrastructure.NoEmail{}, loadDirInMemoryFs("testdata/library"), defaultTestConfig())
	t.Cleanup(func() { _ = app.Shutdown() })
	documents := []model.AugmentedDocument{{Document: index.Document{
		Slug: "book", ID: "book.epub", Metadata: metadata.Metadata{Title: "Book", Format: "EPUB"},
	}}}
	app.Get("/test-delete-button", func(c fiber.Ctx) error {
		return c.Render("partials/"+c.Query("partial"), fiber.Map{
			"Session":              model.Session{User: model.User{Role: model.RoleAdmin}},
			"Results":              result.NewPaginated(10, 1, 1, documents),
			"Paginator":            fiber.Map{"Pages": []int{}},
			"IndexingProgressKind": c.Query("phase"),
		})
	})
	for _, partial := range []string{"docs-list-content", "highlights-list"} {
		for _, phase := range []string{"", "documents", "authors", "textrank", "pruning"} {
			t.Run(partial+"/"+phase, func(t *testing.T) {
				response, err := app.Test(mustGetRequest(t, "/test-delete-button?partial="+partial+"&phase="+phase))
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					t.Fatalf("status = %d", response.StatusCode)
				}
				page, err := goquery.NewDocumentFromReader(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				button := page.Find("button[data-url='/documents/book']")
				if button.Length() != 1 || button.Is("[disabled]") != (phase == "documents") {
					t.Fatalf("incorrect delete button state for phase %q", phase)
				}
			})
		}
	}
}

func TestUpload(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	appFS := loadDirInMemoryFs("testdata/library")
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

		file, err := os.ReadFile("testdata/upload/haruko-html-jpeg.epub")
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
		readers := map[string]metadata.Reader{
			".epub": metadata.NewEpubReader(),
			".pdf":  metadata.PdfReader{Fs: fs},
		}
		app := bootstrapApp(db, &infrastructure.NoEmail{}, fs, webserver.Config{}, readers)

		t.Cleanup(func() {
			fs.Remove(filepath.Join(testLibraryDir, "childrens-literature.epub"))
		})

		var buf bytes.Buffer
		multipartWriter := multipart.NewWriter(&buf)

		file, err := os.ReadFile("testdata/upload/childrens-literature.epub")
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

		if response.StatusCode != http.StatusFound && response.StatusCode != http.StatusSeeOther {
			t.Errorf("Expected status 302 or 303, got %d", response.StatusCode)
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
