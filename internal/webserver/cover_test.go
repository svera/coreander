package webserver_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

func coverTestPage(t *testing.T, app *fiber.App, req *http.Request) *goquery.Document {
	t.Helper()
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s status = %d, want %d", req.URL, resp.StatusCode, http.StatusOK)
	}
	page, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestPDFCoverRejectsStaleCache(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	fs := loadFilesInMemoryFs([]string{"testdata/library/metadata.pdf"})
	cfg := defaultTestConfig()
	cfg.CacheDir = "cache"
	if err := fs.MkdirAll("cache/covers", 0755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "cache/covers/john-doe-test-pdf.webp", []byte("wrong interior image"), 0644); err != nil {
		t.Fatal(err)
	}
	app := bootstrapApp(db, &infrastructure.NoEmail{}, fs, cfg)
	t.Cleanup(func() { _ = app.Shutdown() })
	resp, err := app.Test(mustGetRequest(t, "/documents/john-doe-test-pdf/cover"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("PDF cover status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestPDFCoverMarkup(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	cfg := defaultTestConfig()
	cfg.CoverMaxWidth = 600
	app := bootstrapApp(db, &infrastructure.NoEmail{},
		loadFilesInMemoryFs([]string{"testdata/library/metadata.pdf", "testdata/library/metadata.epub"}), cfg)
	t.Cleanup(func() { _ = app.Shutdown() })
	for _, tc := range []struct {
		path  string
		epub  bool
		eager bool
	}{
		{path: "/search?search=Test"},
		{path: "/documents/john-doe-test-pdf", eager: true},
		{path: "/documents/john-doe-test-epub", epub: true, eager: true},
	} {
		t.Run(tc.path, func(t *testing.T) {
			page := coverTestPage(t, app, mustGetRequest(t, tc.path))
			if width, _ := page.Find("meta[name='cover-max-width']").Attr("content"); width != "600" {
				t.Fatalf("cover width = %q", width)
			}
			if tc.epub {
				if src, _ := page.Find("img.cover").First().Attr("src"); src != "/documents/john-doe-test-epub/cover" {
					t.Fatalf("EPUB cover src = %q", src)
				}
				return
			}
			img := page.Find("img[data-pdf-src='/documents/john-doe-test-pdf/download']").First()
			if img.Length() != 1 {
				t.Fatal("missing PDF page-rendered cover")
			}
			if src, _ := img.Attr("src"); src == "/documents/john-doe-test-pdf/cover" {
				t.Fatal("PDF still loads extracted embedded images")
			}
			_, eager := img.Attr("data-cover-eager")
			if eager != tc.eager {
				t.Fatalf("eager = %v, want %v", eager, tc.eager)
			}
			if !eager {
				if loading, _ := img.Attr("loading"); loading != "lazy" {
					t.Fatalf("loading = %q, want lazy", loading)
				}
			}
		})
	}
}

func TestCoverAspectRatio(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	app := bootstrapApp(db, &infrastructure.NoEmail{},
		loadFilesInMemoryFs([]string{"testdata/library/metadata.pdf", "testdata/library/metadata.epub", "testdata/library/quijote.epub", "testdata/library/empty.pdf"}), defaultTestConfig())
	t.Cleanup(func() { _ = app.Shutdown() })
	cookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	var user model.User
	if err := db.Where("email = ?", "admin@example.com").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	completedOn := time.Now()
	for _, slug := range []string{"john-doe-test-pdf", "john-doe-test-epub"} {
		if err := db.Create(&model.Highlight{UserID: int(user.ID), Slug: slug}).Error; err != nil {
			t.Fatal(err)
		}
		req := mustGetRequest(t, "/documents/"+slug+"/read")
		req.AddCookie(cookie)
		coverTestPage(t, app, req)
	}
	for _, slug := range []string{"miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha", "sergio-vera-empty"} {
		if err := db.Create(&model.Reading{UserID: int(user.ID), Slug: slug, CompletedOn: &completedOn}).Error; err != nil {
			t.Fatal(err)
		}
	}
	docs := []model.AugmentedDocument{
		{Document: index.Document{Slug: "john-doe-test-pdf", Metadata: metadata.Metadata{Title: "Test PDF", Format: "PDF"}}},
		{Document: index.Document{Slug: "john-doe-test-epub", Metadata: metadata.Metadata{Title: "Test EPUB", Format: "EPUB"}}},
	}
	app.Get("/test-related-covers", func(c fiber.Ctx) error {
		return c.Render("partials/document-similar", fiber.Map{"SimilarDocuments": docs, "DocumentSlug": "test"})
	})

	// Reindexed fixtures have no AddedOn date, so supply latest additions explicitly.
	app.Get("/test-cover-home", func(c fiber.Ctx) error {
		return c.Render("index", fiber.Map{"LatestDocs": docs, "Reading": docs}, "layout")
	})

	for _, tc := range []struct {
		name       string
		path       string
		selector   string
		fixed      bool
		count      int
		htmx       bool
		readerLink bool
	}{
		{"latest additions", "/test-cover-home", "#latest-docs img.cover", true, 2, false, false},
		{"compact resume reading", "/test-cover-home", "#in-progress-docs img.cover", true, 2, false, true},
		{"home without latest additions", "/", "#in-progress-docs img.cover", true, 4, false, true},
		{"refreshed compact resume reading", "/resume-reading?compact=true", "#in-progress-docs img.cover", true, 2, false, true},
		{"mobile resume reading", "/resume-reading?compact=false", "#resume-reading-full-carousel img.cover", true, 2, false, true},
		{"desktop resume reading", "/resume-reading?compact=false", "#resume-reading-docs > .row img.cover", true, 2, false, true},
		{"search", "/search?search=Test", "#list img.cover", false, 2, false, false},
		{"PDF detail", "/documents/john-doe-test-pdf", "figure img.cover", false, 1, false, false},
		{"eager EPUB detail", "/documents/john-doe-test-epub", "figure img.cover", false, 1, false, false},
		{"related documents", "/test-related-covers", "img.cover", true, 2, false, false},
		{"highlighted documents block", "/highlights?view=latest", "img.cover", true, 2, false, false},
		{"completed documents page", "/completed?year=0", "#list img.cover", true, 2, false, false},
		{"completed documents refresh", "/completed?year=0", "img.cover", true, 2, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := mustGetRequest(t, tc.path)
			req.AddCookie(cookie)
			if tc.htmx {
				req.Header.Set("HX-Request", "true")
			}
			page := coverTestPage(t, app, req)
			covers := page.Find(tc.selector)
			if covers.Length() != tc.count {
				t.Fatalf("cover count = %d, want %d", covers.Length(), tc.count)
			}
			covers.Each(func(i int, cover *goquery.Selection) {
				if fixed := cover.Closest("figure").HasClass("cover-fixed-ratio"); fixed != tc.fixed {
					t.Errorf("cover %d fixed ratio = %v, want %v", i, fixed, tc.fixed)
				}
				if tc.readerLink {
					if href, _ := cover.Closest("a").Attr("href"); href != "/documents/john-doe-test-pdf/read" && href != "/documents/john-doe-test-epub/read" {
						t.Errorf("resume cover links to %q instead of reader", href)
					}
					if cover.Closest("figure").Find("progress").Length() != 1 {
						t.Error("resume cover is missing reading progress")
					}
				}
			})
		})
	}
}
