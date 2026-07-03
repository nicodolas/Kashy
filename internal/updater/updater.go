// Package updater checks for new Kashy releases on GitHub and performs self-update.
package updater

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// githubReleasesURL is the GitHub API endpoint for the latest release.
	githubReleasesURL = "https://api.github.com/repos/nicodolas/kashy/releases/latest"
	// checkTimeout is the max time to wait for the GitHub API response.
	checkTimeout = 3 * time.Second
)

// Asset represents a file attached to a GitHub release.
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

// releaseResponse is the GitHub API response for the latest release.
type releaseResponse struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// CheckResult holds the outcome of a version check.
type CheckResult struct {
	Available bool   // true if a newer version exists
	Version   string // the latest version tag (e.g. "v1.2.0")
	Assets    []Asset
}

// CheckLatest queries GitHub for the latest release and compares to currentVersion.
// Returns quickly (within checkTimeout). On any error, returns the error so the
// caller can decide to skip the check silently.
func CheckLatest(currentVersion string) (CheckResult, error) {
	return checkLatestFromURL(githubReleasesURL, currentVersion)
}

// checkLatestFromURL is the testable version of CheckLatest with a custom URL.
func checkLatestFromURL(apiURL, currentVersion string) (CheckResult, error) {
	client := &http.Client{Timeout: checkTimeout}
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return CheckResult{}, fmt.Errorf("updater: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return CheckResult{}, fmt.Errorf("updater: request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return CheckResult{}, fmt.Errorf("updater: read body: %w", err)
	}

	var release releaseResponse
	if err := json.Unmarshal(body, &release); err != nil {
		return CheckResult{}, fmt.Errorf("updater: parse response: %w", err)
	}

	if release.TagName == "" {
		return CheckResult{}, fmt.Errorf("updater: empty tag_name in response")
	}

	return CheckResult{
		Available: isNewer(release.TagName, currentVersion),
		Version:   release.TagName,
		Assets:    release.Assets,
	}, nil
}

// isNewer returns true if latest is a higher semver than current.
// Both are expected in "vX.Y.Z" format. Falls back to string comparison on parse error.
func isNewer(latest, current string) bool {
	lv := parseSemver(latest)
	cv := parseSemver(current)
	if lv[0] != cv[0] {
		return lv[0] > cv[0]
	}
	if lv[1] != cv[1] {
		return lv[1] > cv[1]
	}
	return lv[2] > cv[2]
}

// parseSemver parses "vX.Y.Z" into [major, minor, patch].
// Returns [0,0,0] on any parse error.
func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		n, _ := strconv.Atoi(p)
		result[i] = n
	}
	return result
}

// assetNameForPlatform returns the expected zip filename for the current OS/arch.
// Matches the naming convention in release.yml: kashy_{version}_{os}_{arch}.zip
func assetNameForPlatform(version string) string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	return fmt.Sprintf("kashy_%s_%s_%s.zip", version, goos, goarch)
}

// binaryName returns the expected binary filename inside the zip.
func binaryName() string {
	if runtime.GOOS == "windows" {
		return "kashy.exe"
	}
	return "kashy"
}

// findAssetURL finds the download URL for the asset matching targetName.
// Returns empty string if not found.
func findAssetURL(assets []Asset, targetName string) string {
	for _, a := range assets {
		if a.Name == targetName {
			return a.DownloadURL
		}
	}
	return ""
}

// downloadAndExtract downloads a zip from url, extracts the kashy binary,
// and writes it to destPath (replacing existing file).
func downloadAndExtract(url, destPath string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("updater: download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("updater: download status %d", resp.StatusCode)
	}

	zipData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("updater: read zip: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("updater: open zip: %w", err)
	}

	target := binaryName()
	for _, f := range zr.File {
		if f.Name != target {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("updater: open zip entry: %w", err)
		}
		defer rc.Close()

		// Write to a temp file first, then rename atomically
		tmpPath := destPath + ".tmp"
		out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return fmt.Errorf("updater: create temp file: %w", err)
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("updater: write binary: %w", err)
		}
		out.Close()

		if err := os.Rename(tmpPath, destPath); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("updater: replace binary: %w", err)
		}
		return nil
	}

	return fmt.Errorf("updater: binary %q not found in zip", target)
}

// SelfUpdate downloads the latest release binary and replaces the running executable.
// currentExe is the path to the current binary (use os.Executable()).
func SelfUpdate(result CheckResult, currentExe string) error {
	assetName := assetNameForPlatform(result.Version)
	downloadURL := findAssetURL(result.Assets, assetName)
	if downloadURL == "" {
		return fmt.Errorf("updater: no asset found for %s (looking for %s)", result.Version, assetName)
	}

	fmt.Printf("[kashy] downloading %s...\n", assetName)
	if err := downloadAndExtract(downloadURL, currentExe); err != nil {
		return fmt.Errorf("updater: %w", err)
	}
	return nil
}
