package updater

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		current  string
		latest   string
		expected bool
	}{
		{"v1.0.0", "v1.0.1", true},
		{"v1.4.0", "v1.5.2", true},
		{"v1.9.9", "v2.0.0", true},
		{"v2.0.0-rc.1", "v2.0.0", true},
		{"v2.0.0-beta.11", "v2.0.0-rc.1", true},
		{"v1.0.0", "v1.0.0+build.2", false},
		{"v1.1.0", "v1.0.0", false},
		{"v2.0.0", "v2.0.0-rc.1", false},
		{"invalid", "also-invalid", false},
	}

	for _, test := range tests {
		if got := IsNewerVersion(test.current, test.latest); got != test.expected {
			t.Errorf("IsNewerVersion(%q, %q) = %v; want %v", test.current, test.latest, got, test.expected)
		}
	}
}

func TestManagerCheckSelectsExactPlatformAsset(t *testing.T) {
	repository := "owner/project"
	expectedName := expectedAssetName("windows", "amd64")
	expectedURL := "https://github.com/" + repository + "/releases/download/v1.8.0/" + expectedName
	digest := strings.Repeat("a", 64)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/repos/"+repository+"/releases/latest" {
			t.Errorf("unexpected API path %q", request.URL.Path)
		}
		if request.Header.Get("Cache-Control") != "no-cache" {
			t.Error("update request must bypass cached release metadata")
		}
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName:     "v1.8.0",
			HTMLURL:     "https://github.com/" + repository + "/releases/tag/v1.8.0",
			PublishedAt: "2026-08-13T00:00:00Z",
			Assets: []releaseAsset{
				{Name: "random_windows_tool.exe", BrowserDownloadURL: "https://github.com/owner/project/releases/download/v1.8.0/random_windows_tool.exe", Digest: "sha256:" + digest, State: "uploaded", Size: 900000},
				{Name: expectedAssetName("windows", "arm64"), BrowserDownloadURL: "https://github.com/owner/project/releases/download/v1.8.0/codedocs_windows_arm64.exe", Digest: "sha256:" + digest, State: "uploaded", Size: 900000},
				{Name: expectedName, BrowserDownloadURL: expectedURL, Digest: "sha256:" + digest, State: "uploaded", Size: 1000000},
			},
		})
	}))
	defer api.Close()

	manager := NewManager("v1.7.8")
	manager.repository = repository
	manager.goos = "windows"
	manager.goarch = "amd64"
	manager.apiBase = api.URL
	manager.checkClient = api.Client()
	defer manager.Close()

	info, err := manager.Check(t.Context())
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if !info.HasUpdate || info.DownloadURL != expectedURL || info.AssetName != expectedName {
		t.Fatalf("unexpected update info: %+v", info)
	}
	if info.SHA256 != digest || !info.IsVerified || info.SizeBytes != 1000000 {
		t.Fatalf("release verification metadata missing: %+v", info)
	}
	if _, ok := manager.compatible[expectedURL]; !ok || len(manager.compatible) != 1 {
		t.Fatalf("only the exact checked asset should be compatible: %+v", manager.compatible)
	}
}

func TestManagerCheckRejectsMissingArchitecture(t *testing.T) {
	repository := "owner/project"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName: "v2.0.0",
			Assets: []releaseAsset{{
				Name:               expectedAssetName("windows", "arm64"),
				BrowserDownloadURL: "https://github.com/owner/project/releases/download/v2.0.0/codedocs_windows_arm64.exe",
				Digest:             "sha256:" + strings.Repeat("a", 64),
				State:              "uploaded",
				Size:               1000000,
			}},
		})
	}))
	defer api.Close()

	manager := NewManager("v1.7.8")
	manager.repository = repository
	manager.goos = "windows"
	manager.goarch = "amd64"
	manager.apiBase = api.URL
	manager.checkClient = api.Client()
	defer manager.Close()

	if _, err := manager.Check(t.Context()); err == nil {
		t.Fatal("expected a missing architecture error")
	}
	if manager.GetProgress().State != StateError {
		t.Fatalf("expected error state, got %+v", manager.GetProgress())
	}
}

func TestManagerDownloadsAndVerifiesPE(t *testing.T) {
	executable := fakePE("amd64", 700000)
	sum := sha256.Sum256(executable)
	digest := hex.EncodeToString(sum[:])
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(executable)
	}))
	defer download.Close()

	targetPath := filepath.Join(t.TempDir(), "codedocs.exe")
	manager := NewManager("v1.7.8")
	manager.goos = "windows"
	manager.goarch = "amd64"
	manager.downloadClient = download.Client()
	manager.executable = func() (string, error) { return targetPath, nil }
	manager.allowDownloadURL = func(downloadURL *url.URL) bool {
		return strings.HasPrefix(downloadURL.String(), download.URL)
	}
	manager.compatible[download.URL] = releaseAsset{
		Name:               expectedAssetName("windows", "amd64"),
		BrowserDownloadURL: download.URL,
		Digest:             "sha256:" + digest,
		State:              "uploaded",
		Size:               int64(len(executable)),
		ReleaseVersion:     "v1.8.0",
	}
	defer manager.Close()

	if err := manager.StartBackgroundDownload(download.URL); err != nil {
		t.Fatalf("StartBackgroundDownload returned error: %v", err)
	}
	progress := waitForTerminalProgress(t, manager)
	if progress.State != StateReady || !progress.Verified || progress.Version != "v1.8.0" {
		t.Fatalf("expected verified ready state, got %+v", progress)
	}
	prepared, err := os.ReadFile(targetPath + ".new")
	if err != nil {
		t.Fatalf("read prepared update: %v", err)
	}
	if !bytes.Equal(prepared, executable) {
		t.Fatal("prepared update differs from downloaded executable")
	}
}

func TestManagerRejectsUnknownOrCorruptDownload(t *testing.T) {
	manager := NewManager("v1.7.8")
	manager.allowDownloadURL = func(*url.URL) bool { return true }
	if err := manager.StartBackgroundDownload("https://example.test/unissued.exe"); err == nil {
		t.Fatal("expected unissued URL to be rejected")
	}
	manager.Close()

	executable := fakePE("amd64", 600000)
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(executable)
	}))
	defer download.Close()
	targetPath := filepath.Join(t.TempDir(), "codedocs.exe")
	manager = NewManager("v1.7.8")
	manager.goos = "windows"
	manager.goarch = "amd64"
	manager.downloadClient = download.Client()
	manager.executable = func() (string, error) { return targetPath, nil }
	manager.allowDownloadURL = func(*url.URL) bool { return true }
	manager.compatible[download.URL] = releaseAsset{
		Name:               expectedAssetName("windows", "amd64"),
		BrowserDownloadURL: download.URL,
		Digest:             "sha256:" + strings.Repeat("0", 64),
		State:              "uploaded",
		Size:               int64(len(executable)),
	}
	defer manager.Close()

	if err := manager.StartBackgroundDownload(download.URL); err != nil {
		t.Fatalf("StartBackgroundDownload returned error: %v", err)
	}
	progress := waitForTerminalProgress(t, manager)
	if progress.State != StateError || !strings.Contains(progress.Error, "SHA-256") {
		t.Fatalf("expected hash verification failure, got %+v", progress)
	}
	if _, err := os.Stat(targetPath + ".new"); !os.IsNotExist(err) {
		t.Fatal("corrupt update must not be published")
	}
}

func TestValidateUpdatePlanRejectsEscapedPaths(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "codedocs")
	plan := updatePlan{
		ParentPID:  42,
		TargetPath: targetPath,
		NewPath:    targetPath + ".new",
		BackupPath: targetPath + ".old",
		HelperPath: targetPath + helperSuffix(),
		PlanPath:   targetPath + ".update.json",
		ReadyPath:  targetPath + ".update.ready",
		WorkingDir: filepath.Dir(targetPath),
		SHA256:     strings.Repeat("a", 64),
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		CreatedAt:  nowTimestamp(),
		ReadyToken: strings.Repeat("a", 64),
	}
	if err := validateUpdatePlan(plan, plan.PlanPath); err != nil {
		t.Fatalf("valid update plan rejected: %v", err)
	}
	plan.NewPath = filepath.Join(filepath.Dir(targetPath), "other.new")
	if err := validateUpdatePlan(plan, plan.PlanPath); err == nil {
		t.Fatal("expected escaped update path to be rejected")
	}
}

func TestGetProgressLegacyAPI(t *testing.T) {
	testProgress := DownloadProgress{State: StateDownloading, Percent: 45, DownloadedBytes: 4500, TotalBytes: 10000}
	setProgress(testProgress)
	got := GetProgress()
	if got.State != StateDownloading || got.Percent != 45 || got.DownloadedBytes != 4500 {
		t.Errorf("setProgress/GetProgress mismatch: got %+v", got)
	}
	setProgress(DownloadProgress{State: StateIdle})
}

func TestCleanupOldFiles(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Skip("os.Executable unavailable")
	}
	for _, suffix := range []string{".old", ".tmp"} {
		if err := os.WriteFile(executable+suffix, []byte("stale"), 0600); err != nil {
			t.Fatalf("create stale artifact: %v", err)
		}
	}
	CleanupOldFiles()
	for _, suffix := range []string{".old", ".tmp"} {
		if _, err := os.Stat(executable + suffix); !os.IsNotExist(err) {
			t.Errorf("CleanupOldFiles did not remove %s", suffix)
		}
	}
}

func waitForTerminalProgress(t *testing.T, manager *Manager) DownloadProgress {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		progress := manager.GetProgress()
		if progress.State == StateReady || progress.State == StateError {
			return progress
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("updater did not reach a terminal state: %+v", manager.GetProgress())
	return DownloadProgress{}
}

func fakePE(goarch string, size int) []byte {
	data := make([]byte, size)
	copy(data, "MZ")
	data[0x3c] = 0x80
	copy(data[0x80:], []byte{'P', 'E', 0, 0})
	machine := uint16(0x8664)
	if goarch == "arm64" {
		machine = 0xaa64
	}
	data[0x84] = byte(machine)
	data[0x85] = byte(machine >> 8)
	for index := 0x100; index < len(data); index++ {
		data[index] = byte(index)
	}
	return data
}
