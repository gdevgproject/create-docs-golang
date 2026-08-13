package bookmarks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxBookmarkDataBytes = 32 << 20

func (manager *Manager) loadUnlocked() (map[string]Bookmark, error) {
	bookmarks, _, primaryErr := readBookmarkFile(manager.filePath)
	if primaryErr == nil {
		return normalizeBookmarks(bookmarks), nil
	}

	backupPath := manager.filePath + ".bak"
	backup, backupData, backupErr := readBookmarkFile(backupPath)
	if backupErr == nil {
		// Restore a missing or corrupt primary without replacing the known-good
		// backup. Recovery failure is non-fatal because the in-memory data is safe.
		_ = writeAtomic(manager.filePath, backupData, 0600)
		return normalizeBookmarks(backup), nil
	}
	if errors.Is(primaryErr, os.ErrNotExist) && errors.Is(backupErr, os.ErrNotExist) {
		return make(map[string]Bookmark), nil
	}
	if errors.Is(primaryErr, os.ErrNotExist) {
		return nil, fmt.Errorf("load bookmark backup: %w", backupErr)
	}
	return nil, fmt.Errorf("load bookmarks: %w (backup unavailable: %v)", primaryErr, backupErr)
}

func (manager *Manager) saveUnlocked(bookmarks map[string]Bookmark) error {
	bookmarks = normalizeBookmarks(bookmarks)
	data, err := json.MarshalIndent(bookmarks, "", "  ")
	if err != nil {
		return fmt.Errorf("encode bookmarks: %w", err)
	}
	if len(data) > maxBookmarkDataBytes {
		return fmt.Errorf("bookmark data exceeds %d MiB", maxBookmarkDataBytes>>20)
	}
	if err := os.MkdirAll(filepath.Dir(manager.filePath), 0700); err != nil {
		return fmt.Errorf("create bookmark directory: %w", err)
	}

	// Preserve only a valid previous generation. A corrupt primary must never
	// overwrite an existing good backup.
	if previous, err := readRawFile(manager.filePath); err == nil && json.Valid(previous) {
		if err := writeAtomic(manager.filePath+".bak", previous, 0600); err != nil {
			return fmt.Errorf("write bookmark backup: %w", err)
		}
	}
	if err := writeAtomic(manager.filePath, data, 0600); err != nil {
		return fmt.Errorf("write bookmarks: %w", err)
	}
	return nil
}

func readBookmarkFile(path string) (map[string]Bookmark, []byte, error) {
	data, err := readRawFile(path)
	if err != nil {
		return nil, nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return make(map[string]Bookmark), data, nil
	}
	var bookmarks map[string]Bookmark
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&bookmarks); err != nil {
		return nil, data, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, data, fmt.Errorf("decode %s: trailing JSON data", filepath.Base(path))
	}
	if bookmarks == nil {
		bookmarks = make(map[string]Bookmark)
	}
	return bookmarks, data, nil
}

func readRawFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxBookmarkDataBytes {
		return nil, fmt.Errorf("%s exceeds %d MiB", filepath.Base(path), maxBookmarkDataBytes>>20)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBookmarkDataBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBookmarkDataBytes {
		return nil, fmt.Errorf("%s exceeds %d MiB", filepath.Base(path), maxBookmarkDataBytes>>20)
	}
	return data, nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	temporaryPath := path + ".tmp"
	_ = os.Remove(temporaryPath)
	file, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	removeTemporary := true
	defer func() {
		_ = file.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := replaceFile(temporaryPath, path); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}

func replaceFile(sourcePath, destinationPath string) error {
	if err := os.Rename(sourcePath, destinationPath); err == nil {
		return nil
	}
	if err := os.Remove(destinationPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(sourcePath, destinationPath)
}

func normalizeBookmarks(input map[string]Bookmark) map[string]Bookmark {
	normalized := make(map[string]Bookmark, len(input))
	for mapID, bookmark := range input {
		bookmark.Path = normalizePath(bookmark.Path)
		if bookmark.Path == "" {
			continue
		}
		if strings.TrimSpace(bookmark.ID) == "" {
			bookmark.ID = strings.TrimSpace(mapID)
		}
		if bookmark.ID == "" {
			bookmark.ID = uuid.New().String()
		}
		if strings.TrimSpace(bookmark.CreatedAt) == "" {
			bookmark.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
		}
		if len(bookmark.History) == 0 && bookmark.LastResult != nil {
			bookmark.History = []LastResult{*bookmark.LastResult}
		}
		syncLastResult(&bookmark)
		normalized[bookmark.ID] = bookmark
	}

	ordered := make([]Bookmark, 0, len(normalized))
	for _, bookmark := range normalized {
		ordered = append(ordered, bookmark)
	}
	sort.SliceStable(ordered, func(left, right int) bool {
		if ordered[left].Order == ordered[right].Order {
			return ordered[left].ID < ordered[right].ID
		}
		return ordered[left].Order < ordered[right].Order
	})
	for order, bookmark := range ordered {
		bookmark.Order = order
		normalized[bookmark.ID] = bookmark
	}
	return normalized
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}
