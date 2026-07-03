package updater

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// makeZip builds an in-memory zip archive with a single file.
func makeZip(t *testing.T, filename string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(filename)
	if err != nil {
		t.Fatalf("zip.Create: %v", err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	return buf.Bytes()
}

// fakeGitHubAPI returns a test server simulating GitHub releases/latest API.
func fakeGitHubAPI(t *testing.T, tagName string, assets []Asset) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(releaseResponse{
			TagName: tagName,
			Assets:  assets,
		})
	}))
}

// ── CheckLatest ──────────────────────────────────────────────────────────────

func TestCheckLatestReturnsNewerVersion(t *testing.T) {
	srv := fakeGitHubAPI(t, "v1.2.0", nil)
	defer srv.Close()

	result, err := checkLatestFromURL(srv.URL, "v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Available {
		t.Error("expected Available=true for newer version")
	}
	if result.Version != "v1.2.0" {
		t.Errorf("Version: got %q, want %q", result.Version, "v1.2.0")
	}
}

func TestCheckLatestSameVersion(t *testing.T) {
	srv := fakeGitHubAPI(t, "v1.0.0", nil)
	defer srv.Close()

	result, err := checkLatestFromURL(srv.URL, "v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Available {
		t.Error("expected Available=false for same version")
	}
}

func TestCheckLatestNetworkError(t *testing.T) {
	_, err := checkLatestFromURL("http://127.0.0.1:1", "v1.0.0")
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestCheckLatestMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{broken"))
	}))
	defer srv.Close()

	_, err := checkLatestFromURL(srv.URL, "v1.0.0")
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

// ── isNewer ───────────────────────────────────────────────────────────────────

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest  string
		current string
		want    bool
	}{
		{"v1.2.0", "v1.0.0", true},
		{"v1.0.1", "v1.0.0", true},
		{"v2.0.0", "v1.9.9", true},
		{"v1.0.0", "v1.0.0", false},
		{"v1.0.0", "v1.2.0", false},
		{"v1.1.0", "v1.1.0", false},
	}
	for _, tt := range tests {
		got := isNewer(tt.latest, tt.current)
		if got != tt.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}

// ── assetNameForPlatform ──────────────────────────────────────────────────────

func TestAssetNameForPlatform(t *testing.T) {
	name := assetNameForPlatform("v1.2.0")
	if name == "" {
		t.Fatal("assetNameForPlatform returned empty string")
	}
	if !strings.Contains(name, "v1.2.0") {
		t.Errorf("asset name should contain version, got: %s", name)
	}
	switch runtime.GOOS {
	case "windows":
		if !strings.Contains(name, "windows") {
			t.Errorf("expected 'windows' in asset name, got: %s", name)
		}
	case "darwin":
		if !strings.Contains(name, "darwin") {
			t.Errorf("expected 'darwin' in asset name, got: %s", name)
		}
	case "linux":
		if !strings.Contains(name, "linux") {
			t.Errorf("expected 'linux' in asset name, got: %s", name)
		}
	}
}

// ── findAssetURL ──────────────────────────────────────────────────────────────

func TestFindAssetURL(t *testing.T) {
	assets := []Asset{
		{Name: "kashy_v1.2.0_windows_amd64.zip", DownloadURL: "https://example.com/win.zip"},
		{Name: "kashy_v1.2.0_linux_amd64.zip", DownloadURL: "https://example.com/linux.zip"},
		{Name: "kashy_v1.2.0_darwin_amd64.zip", DownloadURL: "https://example.com/mac.zip"},
	}
	target := assetNameForPlatform("v1.2.0")
	url := findAssetURL(assets, target)
	if url == "" {
		t.Errorf("findAssetURL returned empty for target %q", target)
	}
}

func TestFindAssetURLNotFound(t *testing.T) {
	assets := []Asset{
		{Name: "some_other_file.zip", DownloadURL: "https://example.com/other.zip"},
	}
	url := findAssetURL(assets, "kashy_v1.2.0_windows_amd64.zip")
	if url != "" {
		t.Errorf("expected empty URL for missing asset, got: %s", url)
	}
}

// ── downloadAndExtract ────────────────────────────────────────────────────────

func TestDownloadAndExtract(t *testing.T) {
	bin := binaryName()
	zipBytes := makeZip(t, bin, []byte("fake-binary-content"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(zipBytes)
	}))
	defer srv.Close()

	destPath := filepath.Join(t.TempDir(), bin)
	if err := downloadAndExtract(srv.URL, destPath); err != nil {
		t.Fatalf("downloadAndExtract error: %v", err)
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("could not read extracted file: %v", err)
	}
	if string(content) != "fake-binary-content" {
		t.Errorf("extracted content: got %q, want %q", string(content), "fake-binary-content")
	}
}

func TestDownloadAndExtractNetworkError(t *testing.T) {
	err := downloadAndExtract("http://127.0.0.1:1", filepath.Join(t.TempDir(), "kashy"))
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestDownloadAndExtractBinaryNotInZip(t *testing.T) {
	// Zip contains wrong filename → should error
	zipBytes := makeZip(t, "wrong_name.exe", []byte("content"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipBytes)
	}))
	defer srv.Close()

	err := downloadAndExtract(srv.URL, filepath.Join(t.TempDir(), binaryName()))
	if err == nil {
		t.Error("expected error when binary not found in zip")
	}
}
