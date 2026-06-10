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
	appFS := loadDirInMemoryFs("testdata/library")
	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	assertDocumentResults(app, t, "john+doe", 4)

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
		{"Remove non existing document slug", "admin@example.com", "admin", "wrong.epub", "wrong-epub", http.StatusNotFound},
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

			if tcase.expectedHTTPStatus == http.StatusOK {
				if _, err := appFS.Stat(tcase.file); !os.IsNotExist(err) {
					t.Errorf("Expected 'file not exist' error when trying to access a file that should have been removed")
				}

				assertDocumentResults(app, t, "john+doe", 3)
				assertAuthorSearchResults(app, t, "john", 1)
				assertAuthorDocuments(app, t, "john-doe", 3)
			}

			if response.StatusCode != tcase.expectedHTTPStatus {
				t.Errorf("Expected status %d, received %d", tcase.expectedHTTPStatus, response.StatusCode)
			}
		})
	}

	t.Run("Remove document removes orphan author", func(t *testing.T) {
		assertAuthorSearchResults(app, t, "sergio", 1)
		assertAuthorDocuments(app, t, "sergio-vera", 1)

		cookie, err := login(app, "admin@example.com", "admin", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		slug := documentSearchFirstSlug(app, t, "sergio+vera")
		response, err := deleteRequest(url.Values{}, cookie, app, fmt.Sprintf("/documents/%s", slug), t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
		}

		if _, err := appFS.Stat("empty.pdf"); !os.IsNotExist(err) {
			t.Errorf("Expected 'file not exist' error when trying to access a file that should have been removed")
		}

		assertDocumentResults(app, t, "sergio+vera", 0)
		assertAuthorSearchResults(app, t, "sergio", 0)
	})
}
