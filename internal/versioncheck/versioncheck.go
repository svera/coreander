package versioncheck

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const (
	defaultReleaseAPIURL = "https://api.github.com/repos/svera/coreander/releases/latest"
	releasesPageURL      = "https://github.com/svera/coreander/releases/latest"
	requestTimeout       = 5 * time.Second
)

var releaseAPIURL = defaultReleaseAPIURL

type releaseResponse struct {
	TagName string `json:"tag_name"`
}

// NotifyIfOutdated fetches the latest stable GitHub release and logs a message when
// the running version is older. Network or parse failures are ignored.
func NotifyIfOutdated(running string) {
	go func() {
		latest, err := fetchLatestReleaseTag()
		if err != nil {
			return
		}
		if isOlder(running, latest) {
			log.Printf(
				"A new version of Coreander is available: %s (you are running %s). Download: %s\n",
				latest,
				displayVersion(running),
				releasesPageURL,
			)
		}
	}()
}

func fetchLatestReleaseTag() (string, error) {
	client := &http.Client{Timeout: requestTimeout}
	req, err := http.NewRequest(http.MethodGet, releaseAPIURL, nil)
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

func displayVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "unknown"
	}
	return version
}
