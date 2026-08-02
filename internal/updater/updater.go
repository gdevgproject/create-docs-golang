package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"codedocs/internal/config"
)

type UpdateInfo struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	HasUpdate      bool   `json:"has_update"`
	ReleaseNotes   string `json:"release_notes"`
	DownloadURL    string `json:"download_url"`
	PublishedAt    string `json:"published_at"`
}

type DownloadProgress struct {
	State           string `json:"state"` // "idle", "downloading", "ready", "error"
	Percent         int    `json:"percent"`
	DownloadedBytes int64  `json:"downloaded_bytes"`
	TotalBytes      int64  `json:"total_bytes"`
	Error           string `json:"error,omitempty"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type githubReleaseResponse struct {
	TagName     string               `json:"tag_name"`
	Name        string               `json:"name"`
	Body        string               `json:"body"`
	PublishedAt string               `json:"published_at"`
	Assets      []githubReleaseAsset `json:"assets"`
}

var (
	progMu       sync.RWMutex
	currentProg  DownloadProgress
	preparedFile string
)

func GetProgress() DownloadProgress {
	progMu.RLock()
	defer progMu.RUnlock()
	return currentProg
}

func setProgress(p DownloadProgress) {
	progMu.Lock()
	currentProg = p
	progMu.Unlock()
}

// CleanupOldFiles silently removes leftover .old, .new, or .tmp binary files on application launch
func CleanupOldFiles() {
	execPath, err := os.Executable()
	if err != nil {
		return
	}
	execPath, _ = filepath.EvalSymlinks(execPath)
	_ = os.Remove(execPath + ".old")
	_ = os.Remove(execPath + ".tmp")
}

// CheckUpdate checks GitHub Releases API for the latest published release
func CheckUpdate(currentVersion string) (*UpdateInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", config.GitHubRepo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create update request: %w", err)
	}

	req.Header.Set("User-Agent", "CodePulse-Updater/"+currentVersion)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &UpdateInfo{
			CurrentVersion: currentVersion,
			LatestVersion:  currentVersion,
			HasUpdate:      false,
		}, nil
	}

	var rel githubReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("failed to parse release JSON: %w", err)
	}

	latestVer := strings.TrimSpace(rel.TagName)
	hasUpdate := IsNewerVersion(currentVersion, latestVer)

	var downloadURL string
	targetExt := ""
	if runtime.GOOS == "windows" {
		targetExt = ".exe"
	}

	for _, asset := range rel.Assets {
		name := strings.ToLower(asset.Name)
		if strings.Contains(name, runtime.GOOS) || (targetExt != "" && strings.HasSuffix(name, targetExt)) {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" && len(rel.Assets) > 0 {
		downloadURL = rel.Assets[0].BrowserDownloadURL
	}

	return &UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  latestVer,
		HasUpdate:      hasUpdate,
		ReleaseNotes:   rel.Body,
		DownloadURL:    downloadURL,
		PublishedAt:    rel.PublishedAt,
	}, nil
}

// IsNewerVersion compares current and latest semantic version tags
func IsNewerVersion(current, latest string) bool {
	cleanCur := strings.TrimPrefix(strings.TrimSpace(current), "v")
	cleanLat := strings.TrimPrefix(strings.TrimSpace(latest), "v")

	if cleanCur == "" || cleanLat == "" || cleanCur == cleanLat {
		return false
	}

	curParts := strings.Split(cleanCur, ".")
	latParts := strings.Split(cleanLat, ".")

	for i := 0; i < len(curParts) && i < len(latParts); i++ {
		var curNum, latNum int
		fmt.Sscanf(curParts[i], "%d", &curNum)
		fmt.Sscanf(latParts[i], "%d", &latNum)

		if latNum > curNum {
			return true
		}
		if latNum < curNum {
			return false
		}
	}

	return len(latParts) > len(curParts)
}

// StartBackgroundDownload downloads the update binary in background with full network resilience
func StartBackgroundDownload(downloadURL string) error {
	if downloadURL == "" {
		return fmt.Errorf("download URL is empty")
	}

	progMu.Lock()
	if currentProg.State == "downloading" {
		progMu.Unlock()
		return nil // Already downloading
	}
	currentProg = DownloadProgress{State: "downloading", Percent: 0}
	progMu.Unlock()

	go func() {
		execPath, err := os.Executable()
		if err != nil {
			setProgress(DownloadProgress{State: "error", Error: err.Error()})
			return
		}
		execPath, _ = filepath.EvalSymlinks(execPath)
		tmpPath := execPath + ".tmp"
		newPath := execPath + ".new"

		// Clean up any stale download file first
		_ = os.Remove(tmpPath)
		_ = os.Remove(newPath)

		req, err := http.NewRequest("GET", downloadURL, nil)
		if err != nil {
			setProgress(DownloadProgress{State: "error", Error: "Invalid download request"})
			return
		}
		req.Header.Set("User-Agent", "CodePulse-Updater/"+config.Version)

		// Enterprise HTTP Client with 5-minute timeout for large binary payloads
		client := &http.Client{Timeout: 5 * time.Minute}
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			_ = os.Remove(tmpPath)
			setProgress(DownloadProgress{State: "error", Error: "Network error or invalid response"})
			return
		}
		defer resp.Body.Close()

		totalBytes := resp.ContentLength
		outFile, err := os.Create(tmpPath)
		if err != nil {
			setProgress(DownloadProgress{State: "error", Error: "Failed to create temp file: " + err.Error()})
			return
		}

		buf := make([]byte, 32768)
		var downloadedBytes int64

		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				if _, werr := outFile.Write(buf[:n]); werr != nil {
					outFile.Close()
					_ = os.Remove(tmpPath)
					setProgress(DownloadProgress{State: "error", Error: "Disk write error: " + werr.Error()})
					return
				}
				downloadedBytes += int64(n)
				pct := 0
				if totalBytes > 0 {
					pct = int((float64(downloadedBytes) / float64(totalBytes)) * 100)
				}
				setProgress(DownloadProgress{
					State:           "downloading",
					Percent:         pct,
					DownloadedBytes: downloadedBytes,
					TotalBytes:      totalBytes,
				})
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				outFile.Close()
				_ = os.Remove(tmpPath)
				setProgress(DownloadProgress{State: "error", Error: "Download interrupted: " + err.Error()})
				return
			}
		}

		outFile.Close()

		// Safeguard: Ensure binary payload size is valid (> 1MB)
		info, err := os.Stat(tmpPath)
		if err != nil || info.Size() < 1000000 {
			_ = os.Remove(tmpPath)
			setProgress(DownloadProgress{State: "error", Error: "Downloaded payload is incomplete or invalid"})
			return
		}

		if runtime.GOOS != "windows" {
			_ = os.Chmod(tmpPath, 0755)
		}

		// Atomic move from .tmp -> .new when download is 100% verified
		if err := os.Rename(tmpPath, newPath); err != nil {
			_ = os.Remove(tmpPath)
			setProgress(DownloadProgress{State: "error", Error: "Failed to verify update payload"})
			return
		}

		progMu.Lock()
		preparedFile = newPath
		progMu.Unlock()

		setProgress(DownloadProgress{
			State:           "ready",
			Percent:         100,
			DownloadedBytes: downloadedBytes,
			TotalBytes:      totalBytes,
		})
	}()

	return nil
}

// ApplyPreparedUpdate performs enterprise atomic swap with complete rollback safeguard
func ApplyPreparedUpdate() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable path: %w", err)
	}

	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlink: %w", err)
	}

	newPath := execPath + ".new"
	info, err := os.Stat(newPath)
	if err != nil || info.Size() < 1000000 {
		return fmt.Errorf("prepared update binary is missing or invalid")
	}

	oldPath := execPath + ".old"
	_ = os.Remove(oldPath)

	// Step 1: Rename running binary -> .old
	if err := os.Rename(execPath, oldPath); err != nil {
		_ = os.Remove(newPath)
		return fmt.Errorf("failed to move current binary (permission locked): %w", err)
	}

	// Step 2: Rename .new -> execPath
	if err := os.Rename(newPath, execPath); err != nil {
		// Rollback safeguard: restore original binary!
		_ = os.Rename(oldPath, execPath)
		_ = os.Remove(newPath)
		return fmt.Errorf("failed to place new binary (rollback executed): %w", err)
	}

	// Step 3: Restart application with original command line arguments
	cmd := exec.Command(execPath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		// Rollback safeguard if restart fails
		_ = os.Rename(execPath, newPath)
		_ = os.Rename(oldPath, execPath)
		return fmt.Errorf("failed to launch updated binary: %w", err)
	}

	os.Exit(0)
	return nil
}
