package webserver_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

func TestAnnotations(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	app := bootstrapApp(db, &infrastructure.NoEmail{},
		loadFilesInMemoryFs([]string{"testdata/library/metadata.epub"}), webserver.Config{})
	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	other := model.User{
		Uuid: "annotation-reader", Name: "Reader", Username: "annotation-reader",
		Email: "annotation-reader@example.com", Password: model.Hash("test"),
		Role: model.RoleRegular, WordsPerMinute: 250,
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	otherCookie, err := login(app, other.Email, "test", t)
	if err != nil {
		t.Fatal(err)
	}
	slug := "john-doe-test-epub"
	request := func(method, slug, body string, cookie *http.Cookie, want int) []byte {
		t.Helper()
		req, err := http.NewRequest(method, "/documents/"+slug+"/annotations", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			t.Fatalf("%s %s: status %d, want %d; body: %s", method, slug, resp.StatusCode, want, raw)
		}
		return raw
	}
	first := `{"cfi":"epubcfi(/6/2!/4/2,/1:0,/1:5)","content":"First phrase","user_id":999}`
	second := `{"cfi":"epubcfi(/6/2!/4/2,/1:6,/1:12)","content":"Second phrase"}`
	t.Run("reader shows annotation sidebar only when logged in", func(t *testing.T) {
		for _, cookie := range []*http.Cookie{nil, adminCookie} {
			req, err := http.NewRequest(http.MethodGet, "/documents/"+slug+"/read", nil)
			if err != nil {
				t.Fatal(err)
			}
			if cookie != nil {
				req.AddCookie(cookie)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			page, err := goquery.NewDocumentFromReader(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("reader status = %d", resp.StatusCode)
			}
			want := 0
			if cookie != nil {
				want = 1
			}
			for _, selector := range []string{"#annotations-button", "#annotations-side-bar", "#annotations-list"} {
				if page.Find(selector).Length() != want {
					t.Fatalf("%s count = %d, want %d", selector, page.Find(selector).Length(), want)
				}
			}
			if cookie != nil && page.Find("#annotations-button + #menu-button").Length() != 1 {
				t.Fatal("annotations icon is not immediately before settings")
			}
		}
	})
	t.Run("requires authentication", func(t *testing.T) {
		request(http.MethodPost, slug, first, nil, http.StatusForbidden)
		request(http.MethodGet, slug, "", nil, http.StatusForbidden)
		request(http.MethodDelete, slug, first, nil, http.StatusForbidden)
	})
	t.Run("rejects invalid input and missing documents", func(t *testing.T) {
		for _, body := range []string{
			`{`, `{}`, `{"cfi":"not-a-cfi","content":"Text"}`,
			`{"cfi":"epubcfi(/6/2)","content":"  "}`,
			`{"cfi":"epubcfi(/6/2)","content":"` + strings.Repeat("x", 65537) + `"}`,
			`{"cfi":"epubcfi(` + strings.Repeat("1", 8192) + `)","content":"Text"}`,
		} {
			request(http.MethodPost, slug, body, adminCookie, http.StatusBadRequest)
		}
		request(http.MethodPost, "missing-document", first, adminCookie, http.StatusNotFound)
		request(http.MethodGet, "missing-document", "", adminCookie, http.StatusNotFound)
	})
	t.Run("persists multiple annotations and upserts duplicates", func(t *testing.T) {
		request(http.MethodPost, slug, first, adminCookie, http.StatusNoContent)
		request(http.MethodPost, slug, second, adminCookie, http.StatusNoContent)
		request(http.MethodPost, slug, strings.Replace(first, "First phrase", "<b>Updated phrase</b>", 1),
			adminCookie, http.StatusNoContent)
		raw := request(http.MethodGet, slug, "", adminCookie, http.StatusOK)
		var annotations []model.Annotation
		if err := json.Unmarshal(raw, &annotations); err != nil {
			t.Fatal(err)
		}
		if len(annotations) != 2 || annotations[0].Content != "<b>Updated phrase</b>" {
			t.Fatalf("unexpected annotations: %+v", annotations)
		}
		if strings.Contains(string(raw), "user_id") || strings.Contains(string(raw), `"slug"`) {
			t.Fatalf("response exposes internal identifiers: %s", raw)
		}
		if annotations[0].CreatedAt.IsZero() || annotations[0].UpdatedAt.IsZero() {
			t.Fatal("missing annotation timestamps")
		}
	})
	t.Run("isolates users", func(t *testing.T) {
		raw := request(http.MethodGet, slug, "", otherCookie, http.StatusOK)
		if string(raw) != "[]" {
			t.Fatalf("another user's annotations returned: %s", raw)
		}
		request(http.MethodPost, slug, first, otherCookie, http.StatusNoContent)
		var count int64
		if err := db.Model(&model.Annotation{}).Where("slug = ?", slug).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 3 {
			t.Fatalf("count = %d, want 3", count)
		}
	})
	t.Run("removes only the current user's selected annotation", func(t *testing.T) {
		for _, body := range []string{`{`, `{}`, `{"cfi":"invalid"}`,
			`{"cfi":"epubcfi(` + strings.Repeat("1", 8192) + `)"}`} {
			request(http.MethodDelete, slug, body, adminCookie, http.StatusBadRequest)
		}
		request(http.MethodDelete, "missing-document", first, adminCookie, http.StatusNotFound)
		request(http.MethodDelete, slug, first, otherCookie, http.StatusNoContent)
		request(http.MethodDelete, slug, first, otherCookie, http.StatusNoContent)
		var rows []model.Annotation
		if err := db.Where("slug = ?", slug).Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("deletion affected another user's annotations: %+v", rows)
		}
		request(http.MethodDelete, slug, first, adminCookie, http.StatusNoContent)
		raw := request(http.MethodGet, slug, "", adminCookie, http.StatusOK)
		var remaining []model.Annotation
		if err := json.Unmarshal(raw, &remaining); err != nil {
			t.Fatal(err)
		}
		if len(remaining) != 1 || remaining[0].Content != "Second phrase" {
			t.Fatalf("wrong annotation deleted: %+v", remaining)
		}
		request(http.MethodPost, slug, first, otherCookie, http.StatusNoContent)
	})
	t.Run("cascades user deletion", func(t *testing.T) {
		if err := db.Delete(&other).Error; err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := db.Model(&model.Annotation{}).Where("user_id = ?", other.ID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("deleted user's annotations remain: %d", count)
		}
	})
	t.Run("removes annotations when document is deleted", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodDelete, "/documents/"+slug, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(adminCookie)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("delete status = %d", resp.StatusCode)
		}
		var count int64
		if err := db.Model(&model.Annotation{}).Where("slug = ?", slug).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("deleted document's annotations remain: %d", count)
		}
	})
}
