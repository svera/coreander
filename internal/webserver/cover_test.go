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
	for _, path := range []string{"/search?keywords=Test", "/documents/john-doe-test-pdf", "/documents/john-doe-test-epub"} {
		t.Run(path, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, path, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			page, err := goquery.NewDocumentFromReader(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if width, _ := page.Find("meta[name='cover-max-width']").Attr("content"); width != "600" {
				t.Fatalf("cover width = %q", width)
			}
			if path == "/documents/john-doe-test-epub" {
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
			if path == "/documents/john-doe-test-pdf" && !eager {
				t.Fatalf("eager = %v for %s", eager, path)
			}
			if !eager {
				if loading, _ := img.Attr("loading"); loading != "lazy" {
					t.Fatalf("loading = %q, want lazy", loading)
				}
			}
		})
	}
}

func TestCoverBlockAspectRatio(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	app := bootstrapApp(db, &infrastructure.NoEmail{},
		loadFilesInMemoryFs([]string{"testdata/library/metadata.pdf", "testdata/library/metadata.epub"}), defaultTestConfig())
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
		if err := db.Create(&model.Reading{UserID: int(user.ID), Slug: slug, CompletedOn: &completedOn}).Error; err != nil {
			t.Fatal(err)
		}
	}
	app.Get("/test-related-covers", func(c fiber.Ctx) error {
		docs := []model.AugmentedDocument{
			{Document: index.Document{Slug: "john-doe-test-pdf", Metadata: metadata.Metadata{Title: "Test PDF", Format: "PDF"}}},
			{Document: index.Document{Slug: "john-doe-test-epub", Metadata: metadata.Metadata{Title: "Test EPUB", Format: "EPUB"}}},
		}
		return c.Render("partials/document-similar", fiber.Map{"SimilarDocuments": docs, "DocumentSlug": "test"})
	})

	for _, tc := range []struct {
		name string
		path string
		htmx bool
	}{
		{"related documents", "/test-related-covers", false},
		{"highlighted documents block", "/highlights?view=latest", false},
		{"completed documents page", "/completed?year=0", false},
		{"completed documents refresh", "/completed?year=0", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := mustGetRequest(t, tc.path)
			req.AddCookie(cookie)
			if tc.htmx {
				req.Header.Set("HX-Request", "true")
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			page, err := goquery.NewDocumentFromReader(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			selector := "img.cover"
			if tc.path == "/completed?year=0" && !tc.htmx {
				selector = "#list img.cover"
			}
			covers := page.Find(selector)
			if covers.Length() != 2 {
				t.Fatalf("cover count = %d, want 2", covers.Length())
			}
			covers.Each(func(i int, cover *goquery.Selection) {
				if !cover.Closest("figure").HasClass("cover-fixed-ratio") {
					t.Errorf("cover %d is missing fixed aspect ratio", i)
				}
			})
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
		loadFilesInMemoryFs([]string{"testdata/library/metadata.pdf", "testdata/library/metadata.epub"}), defaultTestConfig())
	t.Cleanup(func() { _ = app.Shutdown() })
	cookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}

	// Reindexed fixtures have no AddedOn date, so supply latest additions explicitly.
	app.Get("/test-cover-home", func(c fiber.Ctx) error {
		docs := []model.AugmentedDocument{
			{Document: index.Document{Slug: "john-doe-test-pdf", Metadata: metadata.Metadata{Title: "Test PDF", Format: "PDF"}}},
			{Document: index.Document{Slug: "john-doe-test-epub", Metadata: metadata.Metadata{Title: "Test EPUB", Format: "EPUB"}}},
		}
		return c.Render("index", fiber.Map{"LatestDocs": docs, "Reading": docs}, "layout")
	})

	for _, slug := range []string{"john-doe-test-pdf", "john-doe-test-epub"} {
		req := mustGetRequest(t, "/documents/"+slug+"/read")
		req.AddCookie(cookie)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("reader status = %d", resp.StatusCode)
		}
	}

	for _, tc := range []struct {
		name     string
		path     string
		selector string
		fixed    bool
		count    int
	}{
		{"latest additions", "/test-cover-home", "#latest-docs img.cover", true, 2},
		{"compact resume reading", "/test-cover-home", "#in-progress-docs img.cover", true, 2},
		{"home without latest additions", "/", "#in-progress-docs img.cover", true, 4},
		{"refreshed compact resume reading", "/resume-reading?compact=true", "#in-progress-docs img.cover", true, 2},
		{"mobile resume reading", "/resume-reading?compact=false", "#resume-reading-full-carousel img.cover", true, 2},
		{"desktop resume reading", "/resume-reading?compact=false", "#resume-reading-docs > .row img.cover", true, 2},
		{"search", "/search?keywords=Test", "#list img.cover", false, 2},
		{"PDF detail", "/documents/john-doe-test-pdf", "figure img.cover", false, 1},
		{"eager EPUB detail", "/documents/john-doe-test-epub", "figure img.cover", false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := mustGetRequest(t, tc.path)
			req.AddCookie(cookie)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			page, err := goquery.NewDocumentFromReader(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			covers := page.Find(tc.selector)
			if covers.Length() != tc.count {
				t.Fatalf("cover count = %d, want %d", covers.Length(), tc.count)
			}
			covers.Each(func(i int, cover *goquery.Selection) {
				if fixed := cover.Closest("figure").HasClass("cover-fixed-ratio"); fixed != tc.fixed {
					t.Errorf("cover %d fixed ratio = %v, want %v", i, fixed, tc.fixed)
				}
				if tc.fixed && tc.name != "latest additions" {
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
