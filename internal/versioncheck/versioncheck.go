package versioncheck

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

const (
	defaultReleaseAPIURL = "https://api.github.com/repos/svera/coreander/releases/latest"
	// ReleasesPageURL is the download page linked from the admin footer notice.
	ReleasesPageURL = "https://github.com/svera/coreander/releases/latest"
	requestTimeout  = 5 * time.Second
	// CheckInterval is how often the running version is compared to GitHub's latest release.
	CheckInterval = 24 * time.Hour
)

type releaseResponse struct {
	TagName string `json:"tag_name"`
}

type releaseFetcher func() (string, error)

// Checker periodically compares the running version to the latest GitHub release.
type Checker struct {
	running string
	fetch   releaseFetcher

	mu       sync.RWMutex
	latest   string
	outdated bool
}

// New creates a checker that polls GitHub once per day.
func New(running string) *Checker {
	return NewWithFetcher(running, defaultReleaseFetcher)
}

// NewWithFetcher creates a checker using a custom release fetcher (for tests).
func NewWithFetcher(running string, fetch releaseFetcher) *Checker {
	if fetch == nil {
		fetch = defaultReleaseFetcher
	}
	return &Checker{running: running, fetch: fetch}
}

// Start runs an immediate check and then checks every CheckInterval until the process exits.
func (c *Checker) Start() {
	go c.run()
}

func (c *Checker) run() {
	c.Refresh()
	ticker := time.NewTicker(CheckInterval)
	defer ticker.Stop()
	for range ticker.C {
		c.Refresh()
	}
}

// Refresh fetches the latest release tag and updates the outdated state.
func (c *Checker) Refresh() {
	latest, err := c.fetch()
	if err != nil {
		return
	}
	outdated := isOlder(c.running, latest)
	c.mu.Lock()
	c.latest = latest
	c.outdated = outdated
	c.mu.Unlock()
}

// Outdated reports whether a newer release exists and returns its tag when it does.
func (c *Checker) Outdated() (latest string, outdated bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest, c.outdated
}

func defaultReleaseFetcher() (string, error) {
	return fetchLatestReleaseTag(defaultReleaseAPIURL)
}

func fetchLatestReleaseTag(apiURL string) (string, error) {
	client := &http.Client{Timeout: requestTimeout}
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "coreander-version-check")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var release releaseResponse
	if err := json.Unmarshal(body, &release); err != nil {
		return "", err
	}
	if release.TagName == "" {
		return "", fmt.Errorf("empty tag_name in release response")
	}
	return release.TagName, nil
}

func canonicalize(version string) (string, bool) {
	version = strings.TrimSpace(version)
	if version == "" || version == "unknown" {
		return "", false
	}
	if i := strings.IndexByte(version, '-'); i > 0 {
		version = version[:i]
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	if !semver.IsValid(version) {
		return "", false
	}
	return semver.Canonical(version), true
}

func isOlder(running, latest string) bool {
	current, ok := canonicalize(running)
	if !ok {
		return false
	}
	remote, ok := canonicalize(latest)
	if !ok {
		return false
	}
	return semver.Compare(current, remote) < 0
}
