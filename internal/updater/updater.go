package updater

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"codedocs/internal/config"
)

const (
	StateIdle        = "idle"
	StateChecking    = "checking"
	StateDownloading = "downloading"
	StateVerifying   = "verifying"
	StateReady       = "ready"
	StateApplying    = "applying"
	StateError       = "error"
)

type UpdateInfo struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	HasUpdate      bool   `json:"has_update"`
	ReleaseNotes   string `json:"release_notes"`
	DownloadURL    string `json:"download_url"`
	PublishedAt    string `json:"published_at"`

	AssetName  string `json:"asset_name,omitempty"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	Platform   string `json:"platform,omitempty"`
	ReleaseURL string `json:"release_url,omitempty"`
	IsVerified bool   `json:"is_verified"`
}

// DownloadProgress preserves every field consumed by legacy clients and adds
// richer state for newer interfaces.
type DownloadProgress struct {
	State           string `json:"state"`
	Percent         int    `json:"percent"`
	DownloadedBytes int64  `json:"downloaded_bytes"`
	TotalBytes      int64  `json:"total_bytes"`
	Error           string `json:"error,omitempty"`

	Message   string `json:"message,omitempty"`
	Version   string `json:"version,omitempty"`
	AssetName string `json:"asset_name,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
	Verified  bool   `json:"verified"`
	CanRetry  bool   `json:"can_retry"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type preparedUpdate struct {
	path     string
	asset    releaseAsset
	sha256   string
	verified bool
}

type Manager struct {
	mu               sync.RWMutex
	currentVersion   string
	repository       string
	goos             string
	goarch           string
	checkClient      httpDoer
	downloadClient   httpDoer
	executable       func() (string, error)
	apiBase          string
	allowDownloadURL func(*url.URL) bool

	progress       DownloadProgress
	compatible     map[string]releaseAsset
	prepared       *preparedUpdate
	downloadCancel context.CancelFunc
	closed         bool
}

func NewManager(currentVersion string) *Manager {
	if strings.TrimSpace(currentVersion) == "" {
		currentVersion = config.Version
	}
	return &Manager{
		currentVersion:   currentVersion,
		repository:       config.GitHubRepo,
		goos:             runtime.GOOS,
		goarch:           runtime.GOARCH,
		checkClient:      newCheckHTTPClient(),
		downloadClient:   newDownloadHTTPClient(),
		executable:       resolvedExecutable,
		apiBase:          githubAPIBase,
		allowDownloadURL: isTrustedDownloadURL,
		progress:         DownloadProgress{State: StateIdle},
		compatible:       make(map[string]releaseAsset),
	}
}

func resolvedExecutable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
}

func (manager *Manager) GetProgress() DownloadProgress {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.progress
}

func (manager *Manager) setProgress(progress DownloadProgress) {
	progress.Percent = max(0, min(100, progress.Percent))
	progress.UpdatedAt = nowTimestamp()
	manager.mu.Lock()
	manager.progress = progress
	manager.mu.Unlock()
}

func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func (manager *Manager) Close() {
	manager.mu.Lock()
	manager.closed = true
	if manager.downloadCancel != nil {
		manager.downloadCancel()
		manager.downloadCancel = nil
	}
	manager.mu.Unlock()
}

// CleanupOldFiles removes only updater artifacts adjacent to the running
// executable. It intentionally never traverses directories or uses broad globs.
func CleanupOldFiles() {
	executable, err := resolvedExecutable()
	if err != nil {
		return
	}
	// During a verified update startup the helper still needs the plan and
	// backup until MarkStartupHealthy publishes the readiness signal.
	if os.Getenv(updateReadyPathEnv) != "" || os.Getenv(updateReadyTokenEnv) != "" {
		return
	}
	for _, suffix := range []string{
		".old",
		".tmp",
		".part",
		".new",
		".update.json",
		".update.json.tmp",
		".update.ready",
		".update.ready.tmp",
		helperSuffix(),
		helperSuffix() + ".part",
	} {
		_ = os.Remove(executable + suffix)
	}
}

var (
	legacyMu      sync.Mutex
	legacyManager = NewManager(config.Version)
)

// CheckUpdate is retained for API/source compatibility with previous releases.
func CheckUpdate(currentVersion string) (*UpdateInfo, error) {
	legacyMu.Lock()
	if legacyManager.currentVersion != currentVersion {
		legacyManager.Close()
		legacyManager = NewManager(currentVersion)
	}
	manager := legacyManager
	legacyMu.Unlock()
	return manager.Check(context.Background())
}

func GetProgress() DownloadProgress {
	legacyMu.Lock()
	manager := legacyManager
	legacyMu.Unlock()
	return manager.GetProgress()
}

func StartBackgroundDownload(downloadURL string) error {
	legacyMu.Lock()
	manager := legacyManager
	legacyMu.Unlock()
	return manager.StartBackgroundDownload(downloadURL)
}

func ApplyPreparedUpdate() error {
	legacyMu.Lock()
	manager := legacyManager
	legacyMu.Unlock()
	return manager.ApplyPreparedUpdate()
}

func setProgress(progress DownloadProgress) {
	legacyMu.Lock()
	manager := legacyManager
	legacyMu.Unlock()
	manager.setProgress(progress)
}

func (manager *Manager) fail(message, version string) {
	manager.setProgress(DownloadProgress{
		State:    StateError,
		Error:    message,
		Message:  message,
		Version:  version,
		CanRetry: true,
	})
}

func (manager *Manager) resolveExecutable() (string, error) {
	path, err := manager.executable()
	if err != nil {
		return "", fmt.Errorf("locate application executable: %w", err)
	}
	return filepath.Abs(filepath.Clean(path))
}
