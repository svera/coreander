package webserver_test

import (
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/blevesearch/bleve/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v3/internal/index"
	"github.com/svera/coreander/v3/internal/metadata"
	"github.com/svera/coreander/v3/internal/webserver"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
	"gorm.io/gorm"
)

func TestGET(t *testing.T) {
	var cases = []struct {
		name           string
		url            string
		expectedStatus int
	}{
		{"Redirect if the user tries to access to the root URL", "/", http.StatusFound},
		{"Page loads successfully if the user tries to access the spanish version", "/es", http.StatusOK},
		{"Page loads successfully if the user tries to access the english version", "/en", http.StatusOK},
		{"Server returns not found if the user tries to access a non-existent URL", "/xx", http.StatusNotFound},
	}

	db := infrastructure.Connect("file::memory:", 250)
	app := bootstrapApp(db, &infrastructure.NoEmail{}, afero.NewMemMapFs())

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, tcase.url, nil)

			body, err := app.Test(req)
			if err != nil {
				t.Errorf("Unexpected error: %v", err.Error())
			}
			if body.StatusCode != tcase.expectedStatus {
				t.Errorf("Wrong status code received, expected %d, got %d", tcase.expectedStatus, body.StatusCode)
			}
		})
	}
}

func bootstrapApp(db *gorm.DB, sender webserver.Sender, appFs afero.Fs) *fiber.App {
	var (
		idx *index.BleveIndexer
	)

	metadataReaders := map[string]metadata.Reader{
		".epub": metadata.EpubReader{},
		".pdf":  metadata.PdfReader{},
	}

	webserverConfig := webserver.Config{
		CoverMaxWidth:         600,
		SessionTimeout:        24 * time.Hour,
		LibraryPath:           "fixtures/library",
		UploadDocumentMaxSize: 1,
	}

	indexFile, err := bleve.NewMemOnly(index.Mapping())
	if err == nil {
		idx = index.NewBleve(indexFile, webserverConfig.LibraryPath, metadataReaders)
	}

	err = idx.AddLibrary(appFs, 100)
	if err != nil {
		log.Fatal(err)
	}

	controllers := webserver.SetupControllers(webserverConfig, db, metadataReaders, idx, sender, appFs)
	app := webserver.New(webserverConfig, controllers)
	return app
}

type SMTPMock struct {
	calledSend         bool
	calledSendDocument bool
	mu                 sync.Mutex
	wg                 sync.WaitGroup
}

func (s *SMTPMock) Send(address, subject, body string) error {
	defer s.wg.Done()

	s.mu.Lock()
	s.calledSend = true
	s.mu.Unlock()
	return nil
}

func (s *SMTPMock) SendDocument(address string, libraryPath string, fileName string) error {
	defer s.wg.Done()

	s.mu.Lock()
	s.calledSendDocument = true
	s.mu.Unlock()
	return nil
}

func (s *SMTPMock) From() string {
	return ""
}

func getRequest(cookie *http.Cookie, app *fiber.App, URL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, URL, nil)
	if err != nil {
		return nil, err
	}
	req.AddCookie(cookie)

	return app.Test(req)
}

func postRequest(data url.Values, cookie *http.Cookie, app *fiber.App, URL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, URL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)

	return app.Test(req)
}

func mustReturnForbiddenAndShowLogin(response *http.Response, t *testing.T) {
	if response.StatusCode != http.StatusForbidden {
		t.Errorf("Expected status %d, received %d", http.StatusForbidden, response.StatusCode)
		return
	}

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := doc.Find("head title").First().Html()
	if err != nil {
		t.Fatal(err)
	}
	if selection != "Login" {
		t.Errorf("Expected login page, received %s", selection)
	}
}

func loadDirInMemoryFs(dir string) afero.Fs {
	var (
		contents map[string][]byte
	)

	appFS := afero.NewMemMapFs()

	filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if entry.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			log.Fatalf("Couldn't open %s", entry.Name())
		}
		_, err = file.Read(contents[path])
		if err != nil {
			log.Fatalf("Couldn't read contents of %s", entry.Name())
		}
		afero.WriteFile(appFS, path, contents[entry.Name()], 0644)
		return nil
	})
	return appFS
}
