package webserver_test

import (
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/datasource/wikidata"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

type progressCountingInfo struct {
	webserver.IndexInfo
	calls atomic.Int64
}

type progressStateInfo struct {
	webserver.IndexInfo
	tasks []index.Progress
	err   error
}

func (i *progressStateInfo) BackgroundProgress() ([]index.Progress, error) {
	return i.tasks, i.err
}

func (i *progressCountingInfo) BackgroundProgress() ([]index.Progress, error) {
	i.calls.Add(1)
	return i.IndexInfo.BackgroundProgress()
}

func TestBackgroundProgressOnlyComputedWhenRendered(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	cfg := defaultTestConfig()
	cfg.ServerStaticCacheTTL = 60
	sender := &infrastructure.NoEmail{}
	appFS := afero.NewMemMapFs()
	unusedApp, idx := bootstrapAppWithIndexer(db, sender, appFS, cfg)
	t.Cleanup(func() { _ = unusedApp.Shutdown() })
	t.Cleanup(func() { _ = idx.Close() })
	source := wikidata.NewWikidataSource(wikidata.Gowikidata{})
	provider := biblioreads.NewReviewProvider(nil, nil)
	controllers := webserver.SetupControllers(cfg, db, idx, sender, appFS, source, nil, source, provider)
	counting := &progressCountingInfo{IndexInfo: idx}
	app := webserver.New(cfg, controllers, sender, counting, &model.UserRepository{DB: db})
	t.Cleanup(func() { _ = app.Shutdown() })
	app.Get("/cached-progress-page", func(c fiber.Ctx) error {
		c.Set("Cache-Time", "60")
		return c.Render("auth/login", fiber.Map{}, "layout")
	})

	for _, path := range []string{
		"/js/feedback.js", "/js/feedback.js", "/css/bootstrap.min.css",
		"/images/male.webp", "/reader-service-worker.js", "/documents/missing/score",
	} {
		response, err := app.Test(mustGetRequest(t, path))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s: unexpected status %d", path, response.StatusCode)
		}
		if counting.calls.Load() != 0 {
			t.Fatalf("%s: progress must not be loaded for assets or unrelated fragments", path)
		}
	}
	for _, path := range []string{"/", "/background-progress", "/sessions/new"} {
		before := counting.calls.Load()
		response, err := app.Test(mustGetRequest(t, path))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK || counting.calls.Load() != before+1 {
			t.Fatalf("%s: expected exactly one progress render", path)
		}
	}
	before := counting.calls.Load()
	for range 2 {
		response, err := app.Test(mustGetRequest(t, "/cached-progress-page"))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("cached page returned status %d", response.StatusCode)
		}
	}
	if counting.calls.Load() != before+1 {
		t.Fatal("cached page responses must not recompute progress")
	}
	cfg.RequireAuth = true
	privateApp := webserver.New(cfg, controllers, sender, counting, &model.UserRepository{DB: db})
	t.Cleanup(func() { _ = privateApp.Shutdown() })
	before = counting.calls.Load()
	response, err := privateApp.Test(mustGetRequest(t, "/sessions/new"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || counting.calls.Load() != before {
		t.Fatal("anonymous sign-in pages must not compute inaccessible progress")
	}
}

func TestBackgroundProgressPollingAccess(t *testing.T) {
	for _, requireAuth := range []bool{false, true} {
		name := "public library"
		if requireAuth {
			name = "private library"
		}
		t.Run(name, func(t *testing.T) {
			db := infrastructure.Connect(":memory:", 250)
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			cfg := defaultTestConfig()
			cfg.RequireAuth = requireAuth
			app, idx := bootstrapAppWithIndexer(db, &infrastructure.SMTPMock{}, afero.NewMemMapFs(), cfg)
			t.Cleanup(func() { _ = idx.Close() })
			t.Cleanup(func() { _ = app.Shutdown() })
			idx.BeginIndexing()

			if err := db.Model(&model.User{}).Where("email = ?", "admin@example.com").Updates(map[string]interface{}{
				"recovery_uuid":        "progress-reset",
				"recovery_valid_until": time.Now().UTC().Add(time.Hour),
			}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.Invitation{
				Email:      "invite@example.com",
				UUID:       "progress-invite",
				ValidUntil: time.Now().UTC().Add(time.Hour),
			}).Error; err != nil {
				t.Fatal(err)
			}

			check := func(path string, cookie *http.Cookie, status int, polling bool) {
				t.Helper()
				response, err := getRequest(cookie, app, path, t)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				if response.StatusCode != status {
					t.Fatalf("%s: expected status %d, got %d", path, status, response.StatusCode)
				}
				page, err := goquery.NewDocumentFromReader(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				banner := page.Find("#background-progress")
				if !polling {
					if page.Find(`[hx-get^="/background-progress"]`).Length() != 0 {
						t.Errorf("%s: inaccessible progress endpoint must not be polled", path)
					}
					return
				}
				if banner.AttrOr("hx-trigger", "") != "every 5s" ||
					banner.AttrOr("hx-get", "") != "/background-progress?l=en" ||
					banner.AttrOr("hx-swap", "") != "outerHTML" ||
					banner.AttrOr("data-error-message", "") == "" {
					t.Errorf("%s: expected live progress polling", path)
				}
				if banner.Find(`[data-progress-kind="documents"]`).Length() != 1 {
					t.Errorf("%s: expected indexing progress", path)
				}

			}

			anonymous := &http.Cookie{}
			for _, path := range []string{
				"/sessions/new", "/recover", "/reset-password?id=progress-reset", "/invite?id=progress-invite",
			} {
				check(path, anonymous, http.StatusOK, !requireAuth)
			}
			status := http.StatusOK
			if requireAuth {
				status = http.StatusForbidden
			}
			for _, path := range []string{"/", "/background-progress"} {
				check(path, anonymous, status, !requireAuth)
				check(path, &http.Cookie{Name: "session", Value: "invalid"}, status, !requireAuth)
			}

			cookie, err := login(app, "admin@example.com", "admin", t)
			if err != nil {
				t.Fatal(err)
			}
			check("/", cookie, http.StatusOK, true)
			check("/background-progress", cookie, http.StatusOK, true)
		})
	}
}

func TestBackgroundProgressPollingIntervalTransitions(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	cfg := defaultTestConfig()
	sender := &infrastructure.NoEmail{}
	appFS := afero.NewMemMapFs()
	unusedApp, idx := bootstrapAppWithIndexer(db, sender, appFS, cfg)
	t.Cleanup(func() { _ = unusedApp.Shutdown() })
	t.Cleanup(func() { _ = idx.Close() })
	source := wikidata.NewWikidataSource(wikidata.Gowikidata{})
	provider := biblioreads.NewReviewProvider(nil, nil)
	controllers := webserver.SetupControllers(cfg, db, idx, sender, appFS, source, nil, source, provider)
	state := &progressStateInfo{IndexInfo: idx}
	app := webserver.New(cfg, controllers, sender, state, &model.UserRepository{DB: db})
	t.Cleanup(func() { _ = app.Shutdown() })

	for _, step := range []struct {
		name    string
		tasks   []index.Progress
		err     error
		trigger string
	}{
		{name: "initially idle", trigger: "every 60s"},
		{name: "work starts", tasks: []index.Progress{{Kind: index.ProgressDocuments, InProgress: true}}, trigger: "every 5s"},
		{name: "work completes", trigger: "every 60s"},
		{name: "progress read fails", err: errors.New("progress unavailable"), trigger: "every 5s"},
		{name: "progress read recovers", trigger: "every 60s"},
	} {
		t.Run(step.name, func(t *testing.T) {
			state.tasks, state.err = step.tasks, step.err
			for _, path := range []string{"/?l=en", "/background-progress?l=en"} {
				page := getPage(t, app, path)
				banner := page.Find("#background-progress")
				if got := banner.AttrOr("hx-trigger", ""); got != step.trigger {
					t.Errorf("%s: trigger = %q, want %q", path, got, step.trigger)
				}
				if banner.AttrOr("hx-get", "") != "/background-progress?l=en" ||
					banner.AttrOr("hx-swap", "") != "outerHTML" {
					t.Errorf("%s: polling endpoint or swap changed", path)
				}
				if (banner.Find(`[role="alert"]`).Length() != 0) != (step.err != nil) {
					t.Errorf("%s: incorrect error alert", path)
				}
			}
		})
	}
}

func TestBackgroundProgressErrorHandlerUsesFeedbackScript(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	app := bootstrapApp(db, &infrastructure.NoEmail{}, afero.NewMemMapFs(), webserver.Config{})
	response, err := app.Test(mustGetRequest(t, "/"))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	page, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if page.Find(`script[src^="/js/feedback.js"]`).Length() != 1 {
		t.Fatal("expected feedback script to be loaded")
	}
	if page.Find(`script[src^="/js/background-progress.js"]`).Length() != 0 {
		t.Fatal("expected no standalone background progress script")
	}
}
