package webserver_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

func TestDocumentUpdateGoodReadsRequiresAdmin(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	app := bootstrapApp(db, &infrastructure.NoEmail{}, loadDirInMemoryFs(testLibraryDir), webserver.Config{})
	t.Cleanup(func() { _ = app.Shutdown() })
	users := &model.UserRepository{DB: db}
	if err := users.Create(&model.User{
		Uuid: "regular-goodreads-user", Name: "Regular user", Username: "regular-goodreads",
		Email: "regular@example.com", Password: model.Hash("regular-password"),
		Role: model.RoleRegular, WordsPerMinute: 250,
	}); err != nil {
		t.Fatal(err)
	}
	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	regularCookie, err := login(app, "regular@example.com", "regular-password", t)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		cookie *http.Cookie
		status int
	}{
		{name: "anonymous", cookie: &http.Cookie{}, status: http.StatusForbidden},
		{name: "regular user", cookie: regularCookie, status: http.StatusForbidden},
		{name: "administrator", cookie: adminCookie, status: http.StatusNoContent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response, err := putRequest(url.Values{
				"bookURL": {"https://www.goodreads.com/book/show/1368314.El_Cercano_Oriente"},
			}, tt.cookie, app, "/documents/john-doe-test-epub/goodreads", t)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			mustReturnStatus(response, tt.status, t)
		})
	}
}
