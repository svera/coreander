package webserver_test

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
	"github.com/svera/coreander/v5/internal/result"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

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
