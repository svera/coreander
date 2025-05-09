package webserver_test

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func TestRemoveDocument(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	appFS := loadDirInMemoryFs("fixtures/library")
	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	assertSearchResults(app, t, "john+doe", 4)

	user := &model.User{
		Uuid:           uuid.NewString(),
		Name:           "regular",
		Email:          "regular@example.com",
		Password:       model.Hash("regular"),
		Role:           model.RoleRegular,
		WordsPerMinute: 50,
	}
	result := db.Create(&user)
	if result.Error != nil {
		log.Fatal("Couldn't create regular user")
	}

	var cases = []struct {
		name               string
		email              string
		password           string
		file               string
		slug               string
		expectedHTTPStatus int
	}{
		{"Remove non existing document slug", "admin@example.com", "admin", "wrong.epub", "wrong-epub", http.StatusBadRequest},
		{"Remove document with a regular user", "regular@example.com", "regular", "metadata.epub", "john-doe-test-epub", http.StatusForbidden},
		{"Remove document with an admin user", "admin@example.com", "admin", "metadata.epub", "john-doe-test-epub", http.StatusOK},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			var (
				response *http.Response
				err      error
			)

			cookie, err := login(app, tcase.email, tcase.password, t)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}

			response, err = deleteRequest(url.Values{}, cookie, app, fmt.Sprintf("/documents/%s", tcase.slug), t)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}

			if tcase.expectedHTTPStatus == http.StatusOK {
				if _, err := appFS.Stat(tcase.file); !os.IsNotExist(err) {
					t.Errorf("Expected 'file not exist' error when trying to access a file that should have been removed")
				}

				assertSearchResults(app, t, "john+doe", 3)
			}

			if response.StatusCode != tcase.expectedHTTPStatus {
				t.Errorf("Expected status %d, received %d", tcase.expectedHTTPStatus, response.StatusCode)
			}
		})
	}
}
