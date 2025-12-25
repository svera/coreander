package webserver_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kovidgoyal/imaging"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func TestAuthorImageUpload(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	appFS := loadDirInMemoryFs("fixtures/library")

	// Set required config fields for authentication to work
	// bootstrapApp only sets defaults if config is completely zero-valued,
	// so we need to set required fields explicitly when providing partial config
	app := bootstrapApp(db, &infrastructure.NoEmail{}, appFS, webserver.Config{
		SessionTimeout:      24 * time.Hour,
		RecoveryTimeout:     2 * time.Hour,
		LibraryPath:         "fixtures/library",
		WordsPerMinute:      250,
		CacheDir:            "/tmp",
		AuthorImageMaxWidth: 500,
		JwtSecret:           []byte("test-secret-key-for-author-image-upload-test"),
	})

	// Create test users
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

	// Create a valid test image using the imaging library
	// This ensures the image can be decoded by the handler
	testImg := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			testImg.Set(x, y, color.RGBA{128, 128, 128, 255})
		}
	}

	var imgBuf bytes.Buffer
	if err := imaging.Encode(&imgBuf, testImg, imaging.JPEG); err != nil {
		t.Fatalf("Failed to encode test image: %v", err)
	}
	testImageData := imgBuf.Bytes()

	createMultipartRequest := func(cookie *http.Cookie, authorSlug string) (*http.Request, error) {
		var buf bytes.Buffer
		multipartWriter := multipart.NewWriter(&buf)

		// Use CreatePart with explicit headers to ensure Content-Type is set correctly
		// This matches the pattern used in upload_test.go
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, "image", "test.jpg"))
		h.Set("Content-Type", "image/jpeg")
		part, err := multipartWriter.CreatePart(h)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(testImageData); err != nil {
			return nil, err
		}

		contentType := multipartWriter.FormDataContentType()
		multipartWriter.Close()

		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("/authors/%s/image", authorSlug), &buf)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept-Language", "en")
		req.Header.Set("Content-Type", contentType)
		if cookie != nil {
			req.AddCookie(cookie)
		}

		return req, nil
	}

	t.Run("Try to upload author image without authentication", func(t *testing.T) {
		req, err := createMultipartRequest(nil, "test-author")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		mustReturnForbiddenAndShowLogin(response, t)
	})

	t.Run("Try to upload author image with regular user session", func(t *testing.T) {
		req, err := createMultipartRequest(regularUserCookie, "test-author")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		mustReturnStatus(response, fiber.StatusForbidden, t)
	})

	t.Run("Upload author image successfully with admin session", func(t *testing.T) {
		req, err := createMultipartRequest(adminCookie, "test-author")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if expectedStatus := http.StatusOK; response.StatusCode != expectedStatus {
			// Read response body to see the error message
			body, _ := io.ReadAll(response.Body)
			t.Errorf("Expected status %d, got %d. Response: %s", expectedStatus, response.StatusCode, string(body))
		}

		// Verify response is JSON with success message
		if response.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", response.Header.Get("Content-Type"))
		}
	})
}
