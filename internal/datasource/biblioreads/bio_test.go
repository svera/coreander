package biblioreads_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
)

const englishBio = `Stephen Edwin King was born the second son of Donald and Nellie Ruth Pillsbury King.<br /><br />After his father left them when Stephen was two, he and his older brother, David, were raised by his mother. Parts of his childhood were spent in
  <a href="https://example.com">Fort Wayne</a>
, Indiana &amp; Stratford.`

func TestNormalizeAuthorID(t *testing.T) {
	source := biblioreads.NewBioSource(nil)
	for _, tc := range []struct {
		input, want string
		valid       bool
	}{
		{"", "", true},
		{" \t ", "", true},
		{" 879.Plato ", "879.Plato", true},
		{"https://www.goodreads.com/author/show/879.Plato/", "879.Plato", true},
		{"http://goodreads.com/author/show/879", "879", true},
		{"0", "", false},
		{"../879", "", false},
		{"879?x=y", "", false},
		{"https://example.com/author/show/879", "", false},
		{"https://www.goodreads.com/book/show/879", "", false},
		{"https://user@www.goodreads.com/author/show/879", "", false},
		{"https://www.goodreads.com/author/show/879?x=y", "", false},
		{"https://www.goodreads.com/author/show/879#profile", "", false},
		{"https://www.goodreads.com/author/show/879/extra", "", false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := source.NormalizeAuthorID(tc.input)
			if (err == nil) != tc.valid || got != tc.want {
				t.Fatalf("NormalizeAuthorID(%q) = %q, %v; want %q, valid=%v", tc.input, got, err, tc.want, tc.valid)
			}
		})
	}
}

type fakeAuthorClient struct {
	desc      string
	err       error
	calls     []string
	responses map[string]string
}

func (f *fakeAuthorClient) Author(_ context.Context, id string) (biblioreads.AuthorResponse, error) {
	f.calls = append(f.calls, id)
	if f.responses != nil {
		return biblioreads.AuthorResponse{Desc: f.responses[id]}, f.err
	}
	return biblioreads.AuthorResponse{Desc: f.desc}, f.err
}

func (f *fakeAuthorClient) RefreshAuthor(ctx context.Context, id string) (biblioreads.AuthorResponse, error) {
	return f.Author(ctx, id)
}

func TestBioRejectsUnsupportedInstance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode(biblioreads.AuthorResponse{
			Version: "BiblioReads v4.5.0", Status: "Received", Desc: englishBio,
		}); err != nil {
			t.Errorf("encode author response: %v", err)
		}
	}))
	defer server.Close()
	source := biblioreads.NewBioSource(newTestClient(biblioreads.WithInstances([]string{server.URL})))
	for _, refresh := range []bool{false, true} {
		text, canonical, err := source.RetrieveBio(context.Background(), "3389.Stephen_King", refresh)
		if text != "" || canonical != "" || !errors.Is(err, biblioreads.ErrUnavailable) {
			t.Fatalf("unsupported instance must not return a biography: %q, %q, %v", text, canonical, err)
		}
	}
}

func TestBioRefreshBypassesResponseCache(t *testing.T) {
	const variantID = "18647721.Plat_n"
	const primaryID = "879.Plato"
	const variantNote = `Librarian Note: This is an alternate name variant profile. See primary profile: <a href="/author/show/879.Plato">Plato</a>`
	for _, tc := range []struct {
		name string
		id   string
	}{
		{"direct", primaryID},
		{"primary-profile", variantID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var fresh atomic.Bool
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				var payload struct {
					QueryURL string `json:"queryURL"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("decode author request: %v", err)
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				desc := englishBio
				if strings.HasSuffix(payload.QueryURL, variantID) {
					desc = variantNote
				} else if fresh.Load() {
					desc += " This biography has been updated."
				}
				if err := json.NewEncoder(w).Encode(biblioreads.AuthorResponse{Version: "BiblioReads v4.5.1", Status: "Received", Desc: desc}); err != nil {
					t.Errorf("encode author response: %v", err)
				}
			}))
			defer server.Close()
			client := newTestClient(biblioreads.WithInstances([]string{server.URL}))
			source := biblioreads.NewBioSource(client)
			profiles := []string{primaryID}
			if tc.id == variantID {
				profiles = []string{variantID, primaryID}
			}
			old, canonical, err := source.RetrieveBio(context.Background(), tc.id, false)
			if err != nil || strings.Contains(old, "updated") || canonical != primaryID {
				t.Fatalf("initial biography: %q, %q, %v", old, canonical, err)
			}
			if hits.Load() != int32(len(profiles)) {
				t.Fatalf("expected initial profile requests, got %d", hits.Load())
			}
			fresh.Store(true)
			text, canonical, err := source.RetrieveBio(context.Background(), tc.id, true)
			if err != nil || !strings.Contains(text, "This biography has been updated.") || canonical != primaryID {
				t.Fatalf("refresh must retrieve the new biography: %q, %q, %v", text, canonical, err)
			}
			if hits.Load() != int32(2*len(profiles)) {
				t.Fatalf("refresh must refetch every profile, got %d requests", hits.Load())
			}
			for _, profile := range profiles {
				response, err := client.Author(context.Background(), profile)
				if err != nil {
					t.Fatal(err)
				}
				if profile == primaryID && !strings.Contains(response.Desc, "updated") {
					t.Fatalf("refreshed response must replace the cache: %q", response.Desc)
				}
			}
			if hits.Load() != int32(2*len(profiles)) {
				t.Fatalf("ordinary lookups must use refreshed cache entries, got %d requests", hits.Load())
			}
		})
	}
}

func TestFailedAuthorRefreshPreservesCachedResponse(t *testing.T) {
	var fail atomic.Bool
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if fail.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := json.NewEncoder(w).Encode(biblioreads.AuthorResponse{Version: "BiblioReads v4.5.1", Status: "Received", Desc: englishBio}); err != nil {
			t.Errorf("encode author response: %v", err)
		}
	}))
	defer server.Close()
	client := newTestClient(biblioreads.WithInstances([]string{server.URL}))
	if _, err := client.Author(context.Background(), "879.Plato"); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	if _, err := client.RefreshAuthor(context.Background(), "879.Plato"); err == nil {
		t.Fatal("a failed refresh must return an error")
	}
	response, err := client.Author(context.Background(), "879.Plato")
	if err != nil || response.Desc != englishBio || hits.Load() != 2 {
		t.Fatalf("a failed refresh must leave the previous cached response intact: %+v, %v, hits %d", response, err, hits.Load())
	}
}

func TestBioPrimaryProfile(t *testing.T) {
	const note = `<i>Librarian Note: This is an alternate name variant profile. See primary profile: <b><a href="https://www.goodreads.com/author/show/879.Plato">Plato</a></b>. There is more than one author by this name; see <a href="/author/show/18648109">Platon</a>.</i>`
	client := &fakeAuthorClient{responses: map[string]string{
		"18647721.Plat_n": note,
		"879.Plato":       englishBio,
	}}
	source := biblioreads.NewBioSource(client)
	if source.CachedBioValid("Librarian Note: This is an alternate name variant profile. See primary profile: Plato.") {
		t.Fatal("cached variant notes must be resolved again")
	}
	text, canonical, err := source.RetrieveBio(context.Background(), "18647721.Plat_n", false)
	if err != nil || canonical != "879.Plato" || !strings.HasPrefix(text, "Stephen Edwin King") {
		t.Fatalf("expected the primary profile bio, got %q, %q, %v", text, canonical, err)
	}
	if !slices.Equal(client.calls, []string{"18647721.Plat_n", "879.Plato"}) {
		t.Fatalf("must follow only the primary profile link, got %v", client.calls)
	}
}

func TestBioPrimaryProfileGuards(t *testing.T) {
	note := func(target string) string {
		return `Librarian Note: This is an alternate name variant profile. See primary profile: <a href="` + target + `">Primary</a>`
	}
	for _, tt := range []struct {
		name      string
		desc      string
		wantError bool
	}{
		{"ordinary-link", `A writer influenced by <a href="/author/show/879.Plato">Plato</a>.`, false},
		{"disambiguation", `There is more than one author by this name. See <a href="/author/show/879.Plato">Plato</a>.`, false},
		{"external", note("https://example.com/author/show/879.Plato"), true},
		{"invalid-path", note("https://www.goodreads.com/book/show/879"), true},
		{"missing-link", "Librarian Note: This is an alternate name variant profile. See primary profile: Plato.", true},
		{"self-loop", note("/author/show/3389.Another_Slug"), true},
		{"cycle", note("/author/show/879.Plato"), true},
		{"redirect-limit", note("/author/show/1"), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeAuthorClient{responses: map[string]string{
				"3389.Stephen_King": tt.desc,
				"879.Plato":         note("/author/show/3389.Stephen_King"),
				"1":                 note("/author/show/2"),
				"2":                 note("/author/show/3"),
				"3":                 note("/author/show/4"),
				"4":                 note("/author/show/5"),
			}}
			source := biblioreads.NewBioSource(client)
			text, canonical, err := source.RetrieveBio(context.Background(), "3389.Stephen_King", false)
			if (err != nil) != tt.wantError {
				t.Fatalf("unexpected resolution error: %v", err)
			}
			if tt.wantError && (text != "" || canonical != "") {
				t.Fatal("failed resolutions must not return a variant bio")
			}
			if !tt.wantError && len(client.calls) != 1 {
				t.Fatal("ordinary author links must not be followed")
			}
		})
	}
}

func TestBioCleanup(t *testing.T) {
	for _, tt := range []struct {
		name, desc, want string
	}{
		{
			name: "paragraphs and links", desc: englishBio,
			want: "Stephen Edwin King was born the second son of Donald and Nellie Ruth Pillsbury King.\n\n" +
				"After his father left them when Stephen was two, he and his older brother, David, were raised by his mother. Parts of his childhood were spent in Fort Wayne, Indiana &amp; Stratford.",
		},
		{
			name: "emphasis",
			desc: `<b onclick="bad()">Isaac Asimov</b> wrote <i>Foundation</i> and <strong>many</strong> <em>books</em>.<script>alert(1)</script>`,
			want: `<b>Isaac Asimov</b> wrote <i>Foundation</i> and <strong>many</strong> <em>books</em>.`,
		},
		{name: "empty", desc: "  <br/> "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := biblioreads.NewBioSource(&fakeAuthorClient{desc: tt.desc})
			text, canonical, err := source.RetrieveBio(context.Background(), "3389.Stephen_King", false)
			if err != nil || text != tt.want || canonical != "3389.Stephen_King" {
				t.Fatalf("unexpected cleaned bio: %q, %q, %v", text, canonical, err)
			}
		})
	}
}
