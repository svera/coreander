package webserver_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
)

func TestPutReadingPositionPersistsPercentageAndGetReturnsIt(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	appFs := loadFilesInMemoryFs([]string{
		"testdata/library/metadata.epub",
		"testdata/library/quijote.epub",
	})
	webserverConfig := webserver.Config{
		SessionTimeout: 24 * time.Hour,
		LibraryPath:    "testdata/library",
		WordsPerMinute: 250,
	}
	app := bootstrapApp(db, &infrastructure.NoEmail{}, appFs, webserverConfig)

	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}

	body := `{"position":"epubcfi(/6/2!/4)","percentage":41}`
	req, _ := http.NewRequest(http.MethodPut, "/documents/"+testDocSlug+"/position", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "en")
	req.AddCookie(adminCookie)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT status %d, want 204", resp.StatusCode)
	}
	_ = resp.Body.Close()

	getReq, _ := http.NewRequest(http.MethodGet, "/documents/"+testDocSlug+"/position", nil)
	getReq.Header.Set("Accept-Language", "en")
	getReq.AddCookie(adminCookie)
	getResp, err := app.Test(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET status %d, want 200", getResp.StatusCode)
	}
	raw, _ := io.ReadAll(getResp.Body)
	var out struct {
		Position   string `json:"position"`
		Percentage int    `json:"percentage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("json: %v body=%s", err, string(raw))
	}
	if out.Position != "epubcfi(/6/2!/4)" {
		t.Fatalf("position %q", out.Position)
	}
	if out.Percentage != 41 {
		t.Fatalf("percentage %d, want 41", out.Percentage)
	}
}

func TestPutReadingPositionPreservesPercentageWhenOmitted(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	appFs := loadFilesInMemoryFs([]string{
		"testdata/library/metadata.epub",
		"testdata/library/quijote.epub",
	})
	webserverConfig := webserver.Config{
		SessionTimeout: 24 * time.Hour,
		LibraryPath:    "testdata/library",
		WordsPerMinute: 250,
	}
	app := bootstrapApp(db, &infrastructure.NoEmail{}, appFs, webserverConfig)

	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}

	seed := `{"position":"epubcfi(/6/2!/4)","percentage":60}`
	req, _ := http.NewRequest(http.MethodPut, "/documents/"+testDocSlug+"/position", strings.NewReader(seed))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "en")
	req.AddCookie(adminCookie)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("seed PUT status %d, want 204", resp.StatusCode)
	}
	_ = resp.Body.Close()

	update := `{"position":"epubcfi(/6/2!/8)"}`
	req, _ = http.NewRequest(http.MethodPut, "/documents/"+testDocSlug+"/position", strings.NewReader(update))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "en")
	req.AddCookie(adminCookie)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("update PUT status %d, want 204", resp.StatusCode)
	}
	_ = resp.Body.Close()

	getReq, _ := http.NewRequest(http.MethodGet, "/documents/"+testDocSlug+"/position", nil)
	getReq.Header.Set("Accept-Language", "en")
	getReq.AddCookie(adminCookie)
	getResp, err := app.Test(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	raw, _ := io.ReadAll(getResp.Body)
	var out struct {
		Position   string `json:"position"`
		Percentage int    `json:"percentage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("json: %v body=%s", err, string(raw))
	}
	if out.Position != "epubcfi(/6/2!/8)" {
		t.Fatalf("position %q", out.Position)
	}
	if out.Percentage != 60 {
		t.Fatalf("percentage %d, want 60 preserved", out.Percentage)
	}
}
