package webserver_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

func TestAuthorUpdateRequiresAdmin(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	app := bootstrapApp(db, &infrastructure.NoEmail{}, loadDirInMemoryFs(testLibraryDir), webserver.Config{})

	usersRepository := &model.UserRepository{DB: db}
	if err := usersRepository.Create(&model.User{
		Uuid:           "regular-user-uuid",
		Name:           "Regular user",
		Username:       "regular-user",
		Email:          "regular@example.com",
		Password:       model.Hash("regular-password"),
		Role:           model.RoleRegular,
		WordsPerMinute: 250,
	}); err != nil {
		t.Fatalf("Could not create regular user: %v", err)
	}

	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatalf("Could not log in as admin: %v", err)
	}
	regularCookie, err := login(app, "regular@example.com", "regular-password", t)
	if err != nil {
		t.Fatalf("Could not log in as regular user: %v", err)
	}

	cases := []struct {
		name           string
		cookie         *http.Cookie
		expectedStatus int
	}{
		{name: "anonymous user", cookie: &http.Cookie{}, expectedStatus: fiber.StatusForbidden},
		{name: "regular user", cookie: regularCookie, expectedStatus: fiber.StatusForbidden},
		{name: "administrator", cookie: adminCookie, expectedStatus: fiber.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response, err := putRequest(url.Values{}, tc.cookie, app, "/authors/john-doe", t)
			if err != nil {
				t.Fatalf("Unexpected request error: %v", err)
			}
			if response.StatusCode != tc.expectedStatus {
				t.Errorf("Expected status %d, received %d", tc.expectedStatus, response.StatusCode)
			}
		})
	}
}

func TestDocumentUpdateDataSourceLinkRequiresAdmin(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	app := bootstrapApp(db, &infrastructure.NoEmail{}, loadDirInMemoryFs(testLibraryDir), webserver.Config{})
	t.Cleanup(func() { _ = app.Shutdown() })
	users := &model.UserRepository{DB: db}
	if err := users.Create(&model.User{
		Uuid: "regular-review-link-user", Name: "Regular user", Username: "regular-review-link",
		Email: "regular@example.com", Password: model.Hash("regular-password"),
		Role: model.RoleRegular, WordsPerMinute: 250,
	}); err != nil {
		t.Fatal(err)
	}
	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	regularCookie, err := login(app, "regular@example.com", "regular-password", t)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		cookie *http.Cookie
		status int
	}{
		{name: "anonymous", cookie: &http.Cookie{}, status: http.StatusForbidden},
		{name: "regular user", cookie: regularCookie, status: http.StatusForbidden},
		{name: "administrator without an editable review provider", cookie: adminCookie, status: http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response, err := putRequest(url.Values{
				"bookURL": {"https://www.goodreads.com/book/show/1368314.El_Cercano_Oriente"},
			}, tt.cookie, app, "/documents/john-doe-test-epub/review-link", t)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			mustReturnStatus(response, tt.status, t)
		})
	}
}
