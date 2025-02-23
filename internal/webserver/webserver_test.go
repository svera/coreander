package webserver_test

import (
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/blevesearch/bleve/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/datasource/wikidata"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"gorm.io/gorm"
)

func TestGET(t *testing.T) {
	var cases = []struct {
		name           string
		url            string
		expectedStatus int
	}{
		{"Redirect if the user tries to access to the root URL", "/", http.StatusOK},
		{"Page loads successfully if the user tries to access the spanish version", "/?l=es", http.StatusOK},
		{"Page loads successfully if the user tries to access the english version", "/?l=en", http.StatusOK},
		{"Server returns not found if the user tries to access a non-existent URL", "/xx", http.StatusNotFound},
	}

	db := infrastructure.Connect(":memory:", 250)
	app := bootstrapApp(db, &infrastructure.NoEmail{}, afero.NewMemMapFs(), webserver.Config{})

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

func bootstrapApp(db *gorm.DB, sender webserver.Sender, appFs afero.Fs, webserverConfig webserver.Config) *fiber.App {
	var (
		idx *index.BleveIndexer
	)

	dataSource := wikidata.NewWikidataSource(wikidata.GowikidataMock{})

	metadataReaders := map[string]metadata.Reader{
		".epub": metadata.NewEpubReader(),
		".pdf":  metadata.PdfReader{},
	}

	if reflect.ValueOf(webserverConfig).IsZero() {
		webserverConfig = webserver.Config{
			SessionTimeout:        24 * time.Hour,
			RecoveryTimeout:       2 * time.Hour,
			LibraryPath:           "fixtures/library",
			UploadDocumentMaxSize: 1,
		}
	}

	indexFile, err := bleve.NewMemOnly(index.CreateMapping())
	if err == nil {
		idx = index.NewBleve(indexFile, appFs, webserverConfig.LibraryPath, metadataReaders)
	}

	err = idx.AddLibrary(100, true)
	if err != nil {
		log.Fatal(err)
	}
	controllers := webserver.SetupControllers(webserverConfig, db, metadataReaders, idx, sender, appFs, dataSource)
	return webserver.New(webserverConfig, controllers, sender, idx)
}

func loadFilesInMemoryFs(files []string) afero.Fs {
	var (
		contents map[string][]byte
	)

	appFS := afero.NewMemMapFs()

	for _, fileName := range files {
		file, err := os.Open(fileName)
		if err != nil {
			log.Fatalf("Couldn't open %s", fileName)
		}
		_, err = file.Read(contents[fileName])
		if err != nil {
			log.Fatalf("Couldn't read contents of %s", fileName)
		}
		afero.WriteFile(appFS, fileName, contents[fileName], 0644)
	}
	return appFS
}

func getRequest(cookie *http.Cookie, app *fiber.App, URL string, t *testing.T) (*http.Response, error) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, URL, nil)
	if err != nil {
		return nil, err
	}
	req.AddCookie(cookie)

	return app.Test(req)
}

func postRequest(data url.Values, cookie *http.Cookie, app *fiber.App, URL string, t *testing.T) (*http.Response, error) {
	t.Helper()

	return formRequest(http.MethodPost, data, cookie, app, URL)
}

func putRequest(data url.Values, cookie *http.Cookie, app *fiber.App, URL string, t *testing.T) (*http.Response, error) {
	t.Helper()

	return formRequest(http.MethodPut, data, cookie, app, URL)
}

func deleteRequest(data url.Values, cookie *http.Cookie, app *fiber.App, URL string, t *testing.T) (*http.Response, error) {
	t.Helper()

	return formRequest(http.MethodDelete, data, cookie, app, URL)
}

func formRequest(method string, data url.Values, cookie *http.Cookie, app *fiber.App, URL string) (*http.Response, error) {
	req, err := http.NewRequest(method, URL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)

	return app.Test(req)
}

func mustReturnForbiddenAndShowLogin(response *http.Response, t *testing.T) {
	t.Helper()

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
	if selection != "Login | Coreander" {
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
