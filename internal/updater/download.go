package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	minExecutableBytes = 512 << 10
	maxExecutableBytes = 512 << 20
	progressInterval   = 100 * time.Millisecond
)

func (manager *Manager) StartBackgroundDownload(downloadURL string) error {
	parsedURL, err := url.Parse(downloadURL)
	if err != nil || !manager.allowDownloadURL(parsedURL) {
		return fmt.Errorf("download URL is not trusted")
	}

	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return fmt.Errorf("updater is closed")
	}
	if manager.progress.State == StateDownloading || manager.progress.State == StateVerifying {
		manager.mu.Unlock()
		return fmt.Errorf("an update download is already in progress")
	}
	if manager.progress.State == StateApplying {
		manager.mu.Unlock()
		return fmt.Errorf("an update is being applied")
	}
	asset, compatible := manager.compatible[downloadURL]
	if !compatible {
		manager.mu.Unlock()
		return fmt.Errorf("download URL was not issued by the update check")
	}
	expectedDigest, hasDigest := asset.sha256()
	if !hasDigest {
		manager.mu.Unlock()
		return fmt.Errorf("release asset is missing a valid SHA-256 digest")
	}
	if asset.Size < minExecutableBytes || asset.Size > maxExecutableBytes {
		manager.mu.Unlock()
		return fmt.Errorf("release asset size is outside the safe range")
	}

	ctx, cancel := context.WithCancel(context.Background())
	manager.downloadCancel = cancel
	manager.prepared = nil
	manager.progress = DownloadProgress{
		State:      StateDownloading,
		TotalBytes: asset.Size,
		Message:    "Downloading " + asset.Name,
		Version:    asset.ReleaseVersion,
		AssetName:  asset.Name,
		SHA256:     expectedDigest,
		UpdatedAt:  nowTimestamp(),
	}
	manager.mu.Unlock()

	go manager.download(ctx, asset)
	return nil
}

func (manager *Manager) download(ctx context.Context, asset releaseAsset) {
	executable, err := manager.resolveExecutable()
	if err != nil {
		manager.finishDownloadError(err)
		return
	}
	temporaryPath := executable + ".tmp"
	preparedPath := executable + ".new"
	_ = os.Remove(temporaryPath)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		manager.finishDownloadError(fmt.Errorf("create download request: %w", err))
		return
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "CodeDocs/"+manager.currentVersion)

	response, err := manager.downloadClient.Do(request)
	if err != nil {
		manager.finishDownloadError(fmt.Errorf("download update: %w", err))
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		manager.finishDownloadError(fmt.Errorf("download server returned %s", response.Status))
		return
	}
	if response.Request != nil && response.Request.URL != nil && !manager.allowDownloadURL(response.Request.URL) {
		manager.finishDownloadError(fmt.Errorf("download redirected to an untrusted host"))
		return
	}
	if response.ContentLength > 0 && response.ContentLength != asset.Size {
		manager.finishDownloadError(fmt.Errorf("download size differs from release metadata"))
		return
	}

	file, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		manager.finishDownloadError(fmt.Errorf("create temporary update: %w", err))
		return
	}
	removeTemporary := true
	defer func() {
		_ = file.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	expectedDigest, _ := asset.sha256()
	hasher := sha256.New()
	writer := bufio.NewWriterSize(io.MultiWriter(file, hasher), 256<<10)
	buffer := make([]byte, 256<<10)
	limited := io.LimitReader(response.Body, asset.Size+1)
	var downloaded int64
	lastProgress := time.Time{}
	for {
		readBytes, readErr := limited.Read(buffer)
		if readBytes > 0 {
			written, writeErr := writer.Write(buffer[:readBytes])
			downloaded += int64(written)
			if writeErr != nil {
				manager.finishDownloadError(fmt.Errorf("write update: %w", writeErr))
				return
			}
			if written != readBytes {
				manager.finishDownloadError(io.ErrShortWrite)
				return
			}
			if time.Since(lastProgress) >= progressInterval {
				manager.reportDownload(asset, expectedDigest, downloaded)
				lastProgress = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			manager.finishDownloadError(fmt.Errorf("read update: %w", readErr))
			return
		}
		if err := ctx.Err(); err != nil {
			manager.finishDownloadError(err)
			return
		}
	}
	if err := writer.Flush(); err != nil {
		manager.finishDownloadError(fmt.Errorf("flush update: %w", err))
		return
	}
	if downloaded != asset.Size {
		manager.finishDownloadError(fmt.Errorf("downloaded %d bytes, expected %d", downloaded, asset.Size))
		return
	}
	if err := file.Sync(); err != nil {
		manager.finishDownloadError(fmt.Errorf("sync update: %w", err))
		return
	}
	if err := file.Close(); err != nil {
		manager.finishDownloadError(fmt.Errorf("close update: %w", err))
		return
	}

	manager.setProgress(DownloadProgress{
		State:           StateVerifying,
		Percent:         96,
		DownloadedBytes: downloaded,
		TotalBytes:      asset.Size,
		Message:         "Verifying update",
		Version:         asset.ReleaseVersion,
		AssetName:       asset.Name,
		SHA256:          expectedDigest,
	})
	actualDigest := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualDigest, expectedDigest) {
		manager.finishDownloadError(fmt.Errorf("SHA-256 verification failed"))
		return
	}
	if err := validateExecutable(temporaryPath, manager.goos, manager.goarch); err != nil {
		manager.finishDownloadError(err)
		return
	}
	_ = os.Remove(preparedPath)
	if err := os.Rename(temporaryPath, preparedPath); err != nil {
		manager.finishDownloadError(fmt.Errorf("prepare verified update: %w", err))
		return
	}
	removeTemporary = false

	manager.mu.Lock()
	if manager.closed {
		manager.downloadCancel = nil
		manager.mu.Unlock()
		_ = os.Remove(preparedPath)
		return
	}
	manager.prepared = &preparedUpdate{
		path:     preparedPath,
		asset:    asset,
		sha256:   actualDigest,
		verified: true,
	}
	manager.downloadCancel = nil
	manager.progress = DownloadProgress{
		State:           StateReady,
		Percent:         100,
		DownloadedBytes: downloaded,
		TotalBytes:      asset.Size,
		Message:         "Update is verified and ready",
		Version:         asset.ReleaseVersion,
		AssetName:       asset.Name,
		SHA256:          actualDigest,
		Verified:        true,
		UpdatedAt:       nowTimestamp(),
	}
	manager.mu.Unlock()
}

func (manager *Manager) reportDownload(asset releaseAsset, digest string, downloaded int64) {
	percent := int(downloaded * 95 / asset.Size)
	manager.mu.Lock()
	if manager.progress.State == StateDownloading {
		manager.progress.Percent = min(95, percent)
		manager.progress.DownloadedBytes = downloaded
		manager.progress.TotalBytes = asset.Size
		manager.progress.AssetName = asset.Name
		manager.progress.SHA256 = digest
		manager.progress.UpdatedAt = nowTimestamp()
	}
	manager.mu.Unlock()
}

func (manager *Manager) finishDownloadError(err error) {
	manager.mu.Lock()
	manager.downloadCancel = nil
	if manager.closed {
		manager.mu.Unlock()
		return
	}
	message := "Update download failed"
	if err != nil {
		message = err.Error()
	}
	manager.progress = DownloadProgress{
		State:     StateError,
		Error:     message,
		Message:   message,
		CanRetry:  true,
		UpdatedAt: nowTimestamp(),
	}
	manager.mu.Unlock()
}
