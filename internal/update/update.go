// Package update provides version update checking for the CNAP CLI.
// It checks the GitHub Releases API in the background and caches results
// for 24 hours to avoid excessive API calls.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cnap-tech/cli/internal/config"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

const (
	repo      = "cnap-tech/cli"
	stateFile = "state.yaml"
)

// ReleaseInfo stores information about a GitHub release.
type ReleaseInfo struct {
	Version     string    `json:"tag_name"`
	URL         string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
}

type stateEntry struct {
	CheckedForUpdateAt time.Time   `yaml:"checked_for_update_at"`
	LatestRelease      ReleaseInfo `yaml:"latest_release"`
}

// ShouldCheckForUpdate returns true if the environment is suitable for update checks.
func ShouldCheckForUpdate() bool {
	if os.Getenv("CNAP_NO_UPDATE_NOTIFIER") != "" {
		return false
	}
	if os.Getenv("CODESPACES") != "" {
		return false
	}
	if isCI() {
		return false
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// CheckForUpdate checks whether a newer version of the CLI is available.
// Returns nil if the check was performed recently (within 24h) or if the
// current version is up to date.
func CheckForUpdate(ctx context.Context, currentVersion string) (*ReleaseInfo, error) {
	stateFilePath, err := statePath()
	if err != nil {
		return nil, err
	}

	// Return early if checked recently
	state, _ := getState(stateFilePath)
	if state != nil && time.Since(state.CheckedForUpdateAt).Hours() < 24 {
		return nil, nil
	}

	// Fetch latest release from GitHub
	release, err := fetchLatestRelease(ctx)
	if err != nil {
		return nil, err
	}

	// Cache the result
	_ = setState(stateFilePath, time.Now(), *release)

	if versionGreaterThan(release.Version, currentVersion) {
		return release, nil
	}

	return nil, nil
}

// IsUnderHomebrew returns true if the CLI binary is managed by Homebrew.
func IsUnderHomebrew() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}

	brewExe, err := exec.LookPath("brew")
	if err != nil {
		return false
	}

	brewPrefixBytes, err := exec.Command(brewExe, "--prefix").Output()
	if err != nil {
		return false
	}

	brewBinPrefix := filepath.Join(strings.TrimSpace(string(brewPrefixBytes)), "bin") + string(filepath.Separator)
	return strings.HasPrefix(exe, brewBinPrefix)
}

// IsRecentRelease returns true if the release was published less than 24 hours ago.
func IsRecentRelease(publishedAt time.Time) bool {
	return !publishedAt.IsZero() && time.Since(publishedAt) < 24*time.Hour
}

func statePath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stateFile), nil
}

func getState(path string) (*stateEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s stateEntry
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func setState(path string, t time.Time, r ReleaseInfo) error {
	data, err := yaml.Marshal(stateEntry{CheckedForUpdateAt: t, LatestRelease: r})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func fetchLatestRelease(ctx context.Context) (*ReleaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected HTTP %d", resp.StatusCode)
	}

	var release ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

// versionGreaterThan returns true if v is a newer version than w.
// Versions are expected as semver strings with optional "v" prefix (e.g. "v0.5.1" or "0.5.1").
func versionGreaterThan(v, w string) bool {
	vParts := parseVersion(v)
	wParts := parseVersion(w)
	if vParts == nil || wParts == nil {
		return false
	}
	for i := 0; i < 3; i++ {
		if vParts[i] > wParts[i] {
			return true
		}
		if vParts[i] < wParts[i] {
			return false
		}
	}
	return false
}

func parseVersion(s string) []int {
	s = strings.TrimPrefix(s, "v")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return nil
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		nums[i] = n
	}
	return nums
}

// isCI returns true if running in a known CI environment.
func isCI() bool {
	return os.Getenv("CI") != "" ||
		os.Getenv("BUILD_NUMBER") != "" ||
		os.Getenv("RUN_ID") != ""
}
