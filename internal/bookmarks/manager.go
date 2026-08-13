package bookmarks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Manager struct {
	filePath string
	mu       sync.RWMutex
}

func NewManager(filePath string) *Manager {
	if strings.TrimSpace(filePath) == "" {
		userConfig, err := os.UserConfigDir()
		if err != nil {
			userConfig = "."
		}
		filePath = filepath.Join(userConfig, "codedocs", "saved_paths.json")
	}
	return &Manager{filePath: filepath.Clean(filePath)}
}

func (manager *Manager) GetBookmarks() (map[string]Bookmark, error) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.loadUnlocked()
}

// SaveBookmark adds a new path or updates the existing bookmark for that path.
func (manager *Manager) SaveBookmark(path, note string) (map[string]Bookmark, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	cleanPath := normalizePath(path)
	if cleanPath == "" {
		return nil, fmt.Errorf("bookmark path is required")
	}
	bookmarks, err := manager.loadUnlocked()
	if err != nil {
		return nil, err
	}
	for id, bookmark := range bookmarks {
		if pathsEqual(bookmark.Path, cleanPath) {
			bookmark.Path = cleanPath
			bookmark.Note = strings.TrimSpace(note)
			bookmarks[id] = bookmark
			return manager.persistUnlocked(bookmarks)
		}
	}
	id := uuid.New().String()
	bookmarks[id] = Bookmark{
		ID:        id,
		Path:      cleanPath,
		Note:      strings.TrimSpace(note),
		Order:     len(bookmarks),
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
		History:   []LastResult{},
	}
	return manager.persistUnlocked(bookmarks)
}

func (manager *Manager) SaveHistoryResult(path string, result LastResult) (map[string]Bookmark, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	cleanPath := normalizePath(path)
	bookmarks, err := manager.loadUnlocked()
	if err != nil {
		return nil, err
	}
	updated := false
	for id, bookmark := range bookmarks {
		if pathsEqual(bookmark.Path, cleanPath) {
			bookmark.History = appendOrUpdateHistory(bookmark.History, result)
			syncLastResult(&bookmark)
			bookmarks[id] = bookmark
			updated = true
		}
	}
	if !updated {
		return bookmarks, nil
	}
	return manager.persistUnlocked(bookmarks)
}

func (manager *Manager) UpdateNote(id, note string) (map[string]Bookmark, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	bookmarks, err := manager.loadUnlocked()
	if err != nil {
		return nil, err
	}
	bookmark, exists := bookmarks[id]
	if !exists {
		return nil, fmt.Errorf("bookmark not found: %s", id)
	}
	bookmark.Note = strings.TrimSpace(note)
	bookmarks[id] = bookmark
	return manager.persistUnlocked(bookmarks)
}

func (manager *Manager) ReorderBookmarks(orderedIDs []string) (map[string]Bookmark, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	bookmarks, err := manager.loadUnlocked()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(orderedIDs))
	nextOrder := 0
	for _, id := range orderedIDs {
		bookmark, exists := bookmarks[id]
		if !exists {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		bookmark.Order = nextOrder
		bookmarks[id] = bookmark
		nextOrder++
	}
	remaining := make([]Bookmark, 0, len(bookmarks)-len(seen))
	for id, bookmark := range bookmarks {
		if _, exists := seen[id]; !exists {
			remaining = append(remaining, bookmark)
		}
	}
	sort.SliceStable(remaining, func(left, right int) bool {
		if remaining[left].Order == remaining[right].Order {
			return remaining[left].ID < remaining[right].ID
		}
		return remaining[left].Order < remaining[right].Order
	})
	for _, bookmark := range remaining {
		bookmark.Order = nextOrder
		bookmarks[bookmark.ID] = bookmark
		nextOrder++
	}
	return manager.persistUnlocked(bookmarks)
}

func (manager *Manager) DeleteBookmark(id string) (map[string]Bookmark, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	bookmarks, err := manager.loadUnlocked()
	if err != nil {
		return nil, err
	}
	delete(bookmarks, id)
	return manager.persistUnlocked(bookmarks)
}

func (manager *Manager) DeleteHistoryItem(bookmarkID, generatedAt string) (map[string]Bookmark, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	bookmarks, err := manager.loadUnlocked()
	if err != nil {
		return nil, err
	}
	bookmark, exists := bookmarks[bookmarkID]
	if !exists {
		return nil, fmt.Errorf("bookmark not found: %s", bookmarkID)
	}
	target := strings.TrimSpace(generatedAt)
	history := make([]LastResult, 0, len(bookmark.History))
	for _, result := range bookmark.History {
		if strings.TrimSpace(result.GeneratedAt) != target {
			history = append(history, result)
		}
	}
	bookmark.History = history
	syncLastResult(&bookmark)
	bookmarks[bookmarkID] = bookmark
	return manager.persistUnlocked(bookmarks)
}

func (manager *Manager) RenameHistoryItem(bookmarkID, generatedAt, label string) (map[string]Bookmark, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	bookmarks, err := manager.loadUnlocked()
	if err != nil {
		return nil, err
	}
	bookmark, exists := bookmarks[bookmarkID]
	if !exists {
		return nil, fmt.Errorf("bookmark not found: %s", bookmarkID)
	}
	target := strings.TrimSpace(generatedAt)
	found := false
	for index := range bookmark.History {
		if strings.TrimSpace(bookmark.History[index].GeneratedAt) == target {
			bookmark.History[index].Label = strings.TrimSpace(label)
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("history item with timestamp %q not found", generatedAt)
	}
	syncLastResult(&bookmark)
	bookmarks[bookmarkID] = bookmark
	return manager.persistUnlocked(bookmarks)
}

func (manager *Manager) ClearHistory(bookmarkID string) (map[string]Bookmark, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	bookmarks, err := manager.loadUnlocked()
	if err != nil {
		return nil, err
	}
	bookmark, exists := bookmarks[bookmarkID]
	if !exists {
		return nil, fmt.Errorf("bookmark not found: %s", bookmarkID)
	}
	bookmark.History = []LastResult{}
	bookmark.LastResult = nil
	bookmarks[bookmarkID] = bookmark
	return manager.persistUnlocked(bookmarks)
}

func (manager *Manager) ExportData() ([]byte, error) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	bookmarks, err := manager.loadUnlocked()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(bookmarks, "", "  ")
}

func (manager *Manager) ImportData(data []byte) (map[string]Bookmark, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(data) == 0 {
		return nil, fmt.Errorf("import payload is empty")
	}
	if len(data) > maxBookmarkDataBytes {
		return nil, fmt.Errorf("import payload exceeds %d MiB", maxBookmarkDataBytes>>20)
	}
	imported, err := decodeImport(data)
	if err != nil {
		return nil, err
	}
	local, err := manager.loadUnlocked()
	if err != nil {
		return nil, err
	}
	for _, incoming := range imported {
		incoming.Path = normalizePath(incoming.Path)
		if incoming.Path == "" {
			continue
		}
		existingID := ""
		for id, bookmark := range local {
			if pathsEqual(bookmark.Path, incoming.Path) {
				existingID = id
				break
			}
		}
		incomingHistory := incoming.History
		if len(incomingHistory) == 0 && incoming.LastResult != nil {
			incomingHistory = []LastResult{*incoming.LastResult}
		}
		if existingID != "" {
			existing := local[existingID]
			if strings.TrimSpace(incoming.Note) != "" {
				existing.Note = strings.TrimSpace(incoming.Note)
			}
			for _, result := range incomingHistory {
				existing.History = appendOrUpdateHistory(existing.History, result)
			}
			syncLastResult(&existing)
			local[existingID] = existing
			continue
		}
		id := uuid.New().String()
		createdAt := strings.TrimSpace(incoming.CreatedAt)
		if createdAt == "" {
			createdAt = time.Now().Format("2006-01-02 15:04:05")
		}
		added := Bookmark{
			ID:        id,
			Path:      incoming.Path,
			Note:      strings.TrimSpace(incoming.Note),
			Order:     len(local),
			CreatedAt: createdAt,
			History:   []LastResult{},
		}
		for _, result := range incomingHistory {
			added.History = appendOrUpdateHistory(added.History, result)
		}
		syncLastResult(&added)
		local[id] = added
	}
	return manager.persistUnlocked(local)
}

func (manager *Manager) persistUnlocked(bookmarks map[string]Bookmark) (map[string]Bookmark, error) {
	bookmarks = normalizeBookmarks(bookmarks)
	if err := manager.saveUnlocked(bookmarks); err != nil {
		return nil, err
	}
	return bookmarks, nil
}

func decodeImport(data []byte) (map[string]Bookmark, error) {
	var importedMap map[string]Bookmark
	if err := json.Unmarshal(data, &importedMap); err == nil && importedMap != nil {
		return importedMap, nil
	}
	var importedSlice []Bookmark
	if err := json.Unmarshal(data, &importedSlice); err != nil {
		return nil, fmt.Errorf("invalid bookmark JSON: %w", err)
	}
	importedMap = make(map[string]Bookmark, len(importedSlice))
	for _, bookmark := range importedSlice {
		id := bookmark.ID
		if id == "" {
			id = uuid.New().String()
		}
		importedMap[id] = bookmark
	}
	return importedMap, nil
}

func pathsEqual(left, right string) bool {
	left = normalizePath(left)
	right = normalizePath(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
