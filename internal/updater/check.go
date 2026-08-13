package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (manager *Manager) Check(ctx context.Context) (*UpdateInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := manager.beginCheck(); err != nil {
		return nil, err
	}

	apiURL := strings.TrimRight(manager.apiBase, "/") + "/repos/" + manager.repository + "/releases/latest"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		manager.fail(err.Error(), "")
		return nil, fmt.Errorf("create update request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("Pragma", "no-cache")
	request.Header.Set("User-Agent", "CodeDocs/"+manager.currentVersion)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	response, err := manager.checkClient.Do(request)
	if err != nil {
		manager.fail("Unable to check for updates", "")
		return nil, fmt.Errorf("check GitHub release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		manager.fail("Update service returned "+response.Status, "")
		return nil, fmt.Errorf("GitHub release API returned %s", response.Status)
	}

	var release githubRelease
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxReleaseBodyBytes+1))
	if err := decoder.Decode(&release); err != nil {
		manager.fail("Update information is invalid", "")
		return nil, fmt.Errorf("decode GitHub release: %w", err)
	}
	if release.Draft || release.Prerelease || strings.TrimSpace(release.TagName) == "" {
		manager.fail("No stable release is available", release.TagName)
		return nil, fmt.Errorf("latest GitHub release is not a stable published release")
	}

	hasUpdate := IsNewerVersion(manager.currentVersion, release.TagName)
	assetName := expectedAssetName(manager.goos, manager.goarch)
	asset, hasAsset := findReleaseAsset(release.Assets, assetName)
	asset.ReleaseVersion = release.TagName
	if hasUpdate && !hasAsset {
		message := fmt.Sprintf("Release %s does not include %s", release.TagName, assetName)
		manager.fail(message, release.TagName)
		return nil, fmt.Errorf("%s", message)
	}

	info := &UpdateInfo{
		CurrentVersion: manager.currentVersion,
		LatestVersion:  release.TagName,
		HasUpdate:      hasUpdate,
		ReleaseNotes:   release.Body,
		PublishedAt:    release.PublishedAt,
		Platform:       manager.goos + "/" + manager.goarch,
		ReleaseURL:     release.HTMLURL,
	}
	compatible := make(map[string]releaseAsset)
	if hasAsset {
		if err := validateReleaseAssetURL(manager.repository, asset.BrowserDownloadURL); err != nil {
			if hasUpdate {
				manager.fail("Release download URL failed validation", release.TagName)
				return nil, err
			}
		} else {
			digest, verified := asset.sha256()
			info.AssetName = asset.Name
			info.SizeBytes = asset.Size
			info.SHA256 = digest
			info.IsVerified = verified
			if hasUpdate {
				info.DownloadURL = asset.BrowserDownloadURL
				compatible[asset.BrowserDownloadURL] = asset
			}
		}
	}

	manager.finishCheck(compatible, release.TagName, hasUpdate)
	return info, nil
}

func (manager *Manager) beginCheck() error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed {
		return fmt.Errorf("updater is closed")
	}
	switch manager.progress.State {
	case StateDownloading, StateVerifying, StateApplying:
		return fmt.Errorf("an update operation is already in progress")
	}
	manager.progress = DownloadProgress{
		State:     StateChecking,
		Message:   "Checking for updates",
		UpdatedAt: nowTimestamp(),
	}
	return nil
}

func (manager *Manager) finishCheck(compatible map[string]releaseAsset, version string, hasUpdate bool) {
	message := "CodeDocs is up to date"
	if hasUpdate {
		message = "Update " + version + " is available"
	}
	manager.mu.Lock()
	manager.compatible = compatible
	manager.prepared = nil
	manager.progress = DownloadProgress{
		State:     StateIdle,
		Message:   message,
		Version:   version,
		CanRetry:  hasUpdate,
		UpdatedAt: nowTimestamp(),
	}
	manager.mu.Unlock()
}

func expectedAssetName(goos, goarch string) string {
	switch goos {
	case "windows":
		return "codedocs_windows_" + goarch + ".exe"
	case "darwin":
		return "codedocs_darwin_" + goarch
	default:
		return "codedocs_" + goos + "_" + goarch
	}
}

func findReleaseAsset(assets []releaseAsset, expectedName string) (releaseAsset, bool) {
	for _, asset := range assets {
		if strings.EqualFold(asset.Name, expectedName) &&
			strings.EqualFold(asset.State, "uploaded") &&
			asset.Size > 0 {
			return asset, true
		}
	}
	return releaseAsset{}, false
}
