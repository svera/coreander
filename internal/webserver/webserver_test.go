package webserver_test

import (
	"log"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/infrastructure"
	"github.com/svera/coreander/internal/metadata"
	"github.com/svera/coreander/internal/webserver"
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
	app := bootstrapApp(db, &infrastructure.NoEmail{})

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

func bootstrapApp(db *gorm.DB, sender webserver.Sender) *fiber.App {
	var (
		idx *index.BleveIndexer
	)

	metadataReaders := map[string]metadata.Reader{
		".epub": metadata.EpubReader{},
		".pdf":  metadata.PdfReader{},
	}

	webserverConfig := webserver.Config{
		CoverMaxWidth:  300,
		SessionTimeout: 24 * time.Hour,
		LibraryPath:    "fixtures",
	}

	indexFile, err := bleve.NewMemOnly(index.Mapping())
	if err == nil {
		idx = index.NewBleve(indexFile, webserverConfig.LibraryPath, metadataReaders)
	}

	err = idx.AddLibrary(afero.NewOsFs(), 100)
	if err != nil {
		log.Fatal(err)
	}

	return webserver.New(idx, webserverConfig, metadataReaders, sender, db)
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
