package bookmarks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type LastResult struct {
	Label       string  `json:"label,omitempty"`
	FileName    string  `json:"file_name"`
	TotalFiles  int     `json:"total"`
	TotalLines  int64   `json:"lines"`
	TotalTokens int64   `json:"tokens"`
	TokenMode   string  `json:"token_mode"`
	SizeBytes   int64   `json:"size"`
	Elapsed     float64 `json:"elapsed"`
	GeneratedAt string  `json:"generated_at"`
}

type Bookmark struct {
	ID         string       `json:"id"`
	Path       string       `json:"path"`
	Note       string       `json:"note"`
	Order      int          `json:"order"`
	CreatedAt  string       `json:"created_at"`
	LastResult *LastResult  `json:"last_result,omitempty"`
	History    []LastResult `json:"history,omitempty"`
}

type Manager struct {
	filePath string
	mu       sync.RWMutex
}

func NewManager(filePath string) *Manager {
	if filePath == "" {
		userConfig, err := os.UserConfigDir()
		if err != nil {
			userConfig = "."
		}
		filePath = filepath.Join(userConfig, "codedocs", "saved_paths.json")
	}

	return &Manager{
		filePath: filePath,
	}
}

// GetBookmarks returns all saved bookmarks as a map, ensuring history sorting
func (m *Manager) GetBookmarks() (map[string]Bookmark, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.loadUnlocked(), nil
}

// SaveBookmark adds or updates a bookmark path with optional note
func (m *Manager) SaveBookmark(path, note string) (map[string]Bookmark, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bookmarks := m.loadUnlocked()
	id := uuid.New().String()

	maxOrder := 0
	for _, bm := range bookmarks {
		if bm.Order >= maxOrder {
			maxOrder = bm.Order + 1
		}
	}

	bookmarks[id] = Bookmark{
		ID:        id,
		Path:      filepath.ToSlash(filepath.Clean(path)),
		Note:      note,
		Order:     maxOrder,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
		History:   []LastResult{},
	}

	if err := m.saveUnlocked(bookmarks); err != nil {
		return nil, err
	}

	return bookmarks, nil
}

// SaveHistoryResult appends a new scan result to a bookmark's timeline if path matches
func (m *Manager) SaveHistoryResult(path string, res LastResult) (map[string]Bookmark, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanTarget := filepath.ToSlash(filepath.Clean(path))
	bookmarks := m.loadUnlocked()

	updated := false
	for id, bm := range bookmarks {
		if filepath.ToSlash(filepath.Clean(bm.Path)) == cleanTarget {
			bm.History = appendOrUpdateHistory(bm.History, res)
			if len(bm.History) > 0 {
				bm.LastResult = &bm.History[0]
			}
			bookmarks[id] = bm
			updated = true
		}
	}

	if updated {
		if err := m.saveUnlocked(bookmarks); err != nil {
			return nil, err
		}
	}

	return bookmarks, nil
}

// UpdateNote modifies the display label of a bookmark
func (m *Manager) UpdateNote(id, note string) (map[string]Bookmark, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bookmarks := m.loadUnlocked()
	bm, exists := bookmarks[id]
	if !exists {
		return nil, fmt.Errorf("bookmark not found: %s", id)
	}

	bm.Note = note
	bookmarks[id] = bm

	if err := m.saveUnlocked(bookmarks); err != nil {
		return nil, err
	}

	return bookmarks, nil
}

// ReorderBookmarks updates sequence ordering for saved bookmarks
func (m *Manager) ReorderBookmarks(orderedIDs []string) (map[string]Bookmark, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bookmarks := m.loadUnlocked()
	for idx, id := range orderedIDs {
		if bm, exists := bookmarks[id]; exists {
			bm.Order = idx
			bookmarks[id] = bm
		}
	}

	if err := m.saveUnlocked(bookmarks); err != nil {
		return nil, err
	}

	return bookmarks, nil
}

// DeleteBookmark removes a bookmark by ID and purges all its associated data
func (m *Manager) DeleteBookmark(id string) (map[string]Bookmark, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bookmarks := m.loadUnlocked()
	delete(bookmarks, id)

	if err := m.saveUnlocked(bookmarks); err != nil {
		return nil, err
	}

	return bookmarks, nil
}

// DeleteHistoryItem removes a specific historical scan entry by timestamp
func (m *Manager) DeleteHistoryItem(bookmarkID, generatedAt string) (map[string]Bookmark, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bookmarks := m.loadUnlocked()
	bm, exists := bookmarks[bookmarkID]
	if !exists {
		return nil, fmt.Errorf("bookmark not found: %s", bookmarkID)
	}

	var newHistory []LastResult
	for _, h := range bm.History {
		if h.GeneratedAt != generatedAt {
			newHistory = append(newHistory, h)
		}
	}
	bm.History = newHistory

	if len(bm.History) > 0 {
		bm.LastResult = &bm.History[0]
	} else {
		bm.LastResult = nil
	}
	bookmarks[bookmarkID] = bm

	if err := m.saveUnlocked(bookmarks); err != nil {
		return nil, err
	}

	return bookmarks, nil
}

// RenameHistoryItem updates the custom label/name of a specific scan history entry
func (m *Manager) RenameHistoryItem(bookmarkID, generatedAt, label string) (map[string]Bookmark, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bookmarks := m.loadUnlocked()
	bm, exists := bookmarks[bookmarkID]
	if !exists {
		return nil, fmt.Errorf("bookmark not found: %s", bookmarkID)
	}

	cleanLabel := strings.TrimSpace(label)
	targetTime := strings.TrimSpace(generatedAt)
	found := false

	for i := range bm.History {
		if strings.TrimSpace(bm.History[i].GeneratedAt) == targetTime {
			bm.History[i].Label = cleanLabel
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("history item with timestamp %q not found", generatedAt)
	}

	// Also sync LastResult if it matches the renamed item
	if bm.LastResult != nil && strings.TrimSpace(bm.LastResult.GeneratedAt) == targetTime {
		bm.LastResult.Label = cleanLabel
	}

	bookmarks[bookmarkID] = bm

	if err := m.saveUnlocked(bookmarks); err != nil {
		return nil, err
	}

	return bookmarks, nil
}

// ClearHistory removes all historical scan records for a given bookmark
func (m *Manager) ClearHistory(bookmarkID string) (map[string]Bookmark, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bookmarks := m.loadUnlocked()
	bm, exists := bookmarks[bookmarkID]
	if !exists {
		return nil, fmt.Errorf("bookmark not found: %s", bookmarkID)
	}

	bm.History = []LastResult{}
	bm.LastResult = nil
	bookmarks[bookmarkID] = bm

	if err := m.saveUnlocked(bookmarks); err != nil {
		return nil, err
	}

	return bookmarks, nil
}

// ExportData exports all bookmarks and complete scan timelines into formatted JSON
func (m *Manager) ExportData() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bookmarks := m.loadUnlocked()
	return json.MarshalIndent(bookmarks, "", "  ")
}

// ImportData merges imported bookmarks with local bookmarks, overwriting matching timestamps
func (m *Manager) ImportData(data []byte) (map[string]Bookmark, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(data) == 0 {
		return nil, fmt.Errorf("import payload is empty")
	}

	// Try unmarshaling as map[string]Bookmark first
	var importedMap map[string]Bookmark
	if err := json.Unmarshal(data, &importedMap); err != nil {
		// Try unmarshaling as []Bookmark slice if array format
		var importedSlice []Bookmark
		if err2 := json.Unmarshal(data, &importedSlice); err2 != nil {
			return nil, fmt.Errorf("invalid json import format: %w", err)
		}
		importedMap = make(map[string]Bookmark, len(importedSlice))
		for _, bm := range importedSlice {
			importedMap[bm.ID] = bm
		}
	}

	localMap := m.loadUnlocked()

	// Track max order for newly added bookmarks
	maxOrder := 0
	for _, bm := range localMap {
		if bm.Order >= maxOrder {
			maxOrder = bm.Order + 1
		}
	}

	for _, impBm := range importedMap {
		cleanImpPath := filepath.ToSlash(filepath.Clean(impBm.Path))
		if cleanImpPath == "" {
			continue
		}

		// Find existing local bookmark matching path
		var existingID string
		var existingBm Bookmark
		found := false

		for lID, lBm := range localMap {
			if filepath.ToSlash(filepath.Clean(lBm.Path)) == cleanImpPath {
				existingID = lID
				existingBm = lBm
				found = true
				break
			}
		}

		// Collect imported history entries (including legacy LastResult)
		impHistory := impBm.History
		if len(impHistory) == 0 && impBm.LastResult != nil {
			impHistory = []LastResult{*impBm.LastResult}
		}

		if found {
			// Cumulative Merge into existing bookmark
			if strings.TrimSpace(impBm.Note) != "" {
				existingBm.Note = impBm.Note
			}
			for _, h := range impHistory {
				existingBm.History = appendOrUpdateHistory(existingBm.History, h)
			}
			if len(existingBm.History) > 0 {
				existingBm.LastResult = &existingBm.History[0]
			}
			localMap[existingID] = existingBm
		} else {
			// Add new bookmark
			newID := uuid.New().String()
			newBm := Bookmark{
				ID:        newID,
				Path:      cleanImpPath,
				Note:      impBm.Note,
				Order:     maxOrder,
				CreatedAt: impBm.CreatedAt,
				History:   []LastResult{},
			}
			if newBm.CreatedAt == "" {
				newBm.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
			}
			for _, h := range impHistory {
				newBm.History = appendOrUpdateHistory(newBm.History, h)
			}
			if len(newBm.History) > 0 {
				newBm.LastResult = &newBm.History[0]
			}
			localMap[newID] = newBm
			maxOrder++
		}
	}

	if err := m.saveUnlocked(localMap); err != nil {
		return nil, err
	}

	return localMap, nil
}

func appendOrUpdateHistory(history []LastResult, res LastResult) []LastResult {
	if res.GeneratedAt == "" {
		res.GeneratedAt = time.Now().Format("2006-01-02 15:04:05")
	}

	foundIdx := -1
	for i, h := range history {
		if h.GeneratedAt == res.GeneratedAt {
			foundIdx = i
			break
		}
	}

	if foundIdx >= 0 {
		// Overwrite duplicate timestamp entry with imported/new payload!
		history[foundIdx] = res
	} else {
		// Append new timestamp entry!
		history = append(history, res)
	}

	// Sort history timeline by GeneratedAt descending (newest scan first)
	sort.Slice(history, func(i, j int) bool {
		return history[i].GeneratedAt > history[j].GeneratedAt
	})

	return history
}

func (m *Manager) loadUnlocked() map[string]Bookmark {
	if _, err := os.Stat(m.filePath); os.IsNotExist(err) {
		return make(map[string]Bookmark)
	}

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return make(map[string]Bookmark)
	}

	if len(data) == 0 {
		return make(map[string]Bookmark)
	}

	var bookmarks map[string]Bookmark
	if err := json.Unmarshal(data, &bookmarks); err != nil {
		return make(map[string]Bookmark)
	}

	// Migrate legacy LastResult to History if needed and ensure sorting
	for id, bm := range bookmarks {
		if len(bm.History) == 0 && bm.LastResult != nil {
			bm.History = []LastResult{*bm.LastResult}
		}
		sort.Slice(bm.History, func(i, j int) bool {
			return bm.History[i].GeneratedAt > bm.History[j].GeneratedAt
		})
		if len(bm.History) > 0 {
			bm.LastResult = &bm.History[0]
		}
		bookmarks[id] = bm
	}

	return bookmarks
}

func (m *Manager) saveUnlocked(bookmarks map[string]Bookmark) error {
	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	data, err := json.MarshalIndent(bookmarks, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal bookmarks: %w", err)
	}

	return os.WriteFile(m.filePath, data, 0644)
}
