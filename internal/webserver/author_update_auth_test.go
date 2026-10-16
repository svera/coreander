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
