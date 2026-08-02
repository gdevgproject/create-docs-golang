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

// CheckUpdate checks GitHub Releases API for the latest published release
func CheckUpdate(currentVersion string) (*UpdateInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", config.GitHubRepo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create update request: %w", err)
	}

	req.Header.Set("User-Agent", "codedocs-updater/"+currentVersion)
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

// ApplyUpdate downloads the new binary, replaces the current running binary, and restarts the app
func ApplyUpdate(downloadURL string) error {
	if downloadURL == "" {
		return fmt.Errorf("download URL is empty")
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable path: %w", err)
	}

	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlink: %w", err)
	}

	// Download new binary to temporary file
	resp, err := http.Get(downloadURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download update binary: %w", err)
	}
	defer resp.Body.Close()

	tmpPath := execPath + ".tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file for update: %w", err)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to save update payload: %w", err)
	}
	tmpFile.Close()

	// Make binary executable on Unix
	if runtime.GOOS != "windows" {
		_ = os.Chmod(tmpPath, 0755)
	}

	// On Windows, rename current running .exe to .exe.old, then rename .tmp to .exe
	oldPath := execPath + ".old"
	_ = os.Remove(oldPath)

	if err := os.Rename(execPath, oldPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to move current binary: %w", err)
	}

	if err := os.Rename(tmpPath, execPath); err != nil {
		// Rollback if failed
		_ = os.Rename(oldPath, execPath)
		return fmt.Errorf("failed to place new binary: %w", err)
	}

	// Restart application with original command line arguments
	cmd := exec.Command(execPath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to restart updated application: %w", err)
	}

	// Exit current process
	os.Exit(0)
	return nil
}
