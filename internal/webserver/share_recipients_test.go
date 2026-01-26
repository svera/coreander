package webserver_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func TestShareRecipientsExcludesSessionUser(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	app := bootstrapApp(db, &infrastructure.NoEmail{}, afero.NewMemMapFs(), webserver.Config{})

	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	regularUserData := url.Values{
		"name":             {"Regular user"},
		"username":         {"regular"},
		"email":            {"regular@example.com"},
		"password":         {"regular"},
		"confirm-password": {"regular"},
		"role":             {fmt.Sprint(model.RoleRegular)},
		"words-per-minute": {"250"},
	}

	response, err := postRequest(regularUserData, adminCookie, app, "/users", t)
	if response == nil || err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	response, err = getRequest(adminCookie, app, "/users/share-recipients", t)
	if response == nil || err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	var recipients []struct {
		Username string `json:"username"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(response.Body).Decode(&recipients); err != nil {
		t.Fatalf("Unexpected error decoding response: %v", err)
	}

	hasAdmin := false
	hasRegular := false
	for _, recipient := range recipients {
		if recipient.Username == "admin" {
			hasAdmin = true
		}
		if recipient.Username == "regular" {
			hasRegular = true
		}
	}

	if hasAdmin {
		t.Error("Expected share recipients to exclude session user")
	}
	if !hasRegular {
		t.Error("Expected share recipients to include other users")
	}
}
