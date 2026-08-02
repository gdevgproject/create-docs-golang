package bookmarks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Bookmark struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
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

// GetBookmarks returns all saved bookmarks as a map
func (m *Manager) GetBookmarks() (map[string]Bookmark, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, err := os.Stat(m.filePath); os.IsNotExist(err) {
		return make(map[string]Bookmark), nil
	}

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read bookmarks file: %w", err)
	}

	if len(data) == 0 {
		return make(map[string]Bookmark), nil
	}

	var bookmarks map[string]Bookmark
	if err := json.Unmarshal(data, &bookmarks); err != nil {
		return make(map[string]Bookmark), nil
	}

	return bookmarks, nil
}

// SaveBookmark adds or updates a bookmark path with optional note
func (m *Manager) SaveBookmark(path, note string) (map[string]Bookmark, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bookmarks := m.loadUnlocked()
	id := uuid.New().String()

	bookmarks[id] = Bookmark{
		ID:        id,
		Path:      filepath.ToSlash(filepath.Clean(path)),
		Note:      note,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
	}

	if err := m.saveUnlocked(bookmarks); err != nil {
		return nil, err
	}

	return bookmarks, nil
}

// DeleteBookmark removes a bookmark by ID
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

func (m *Manager) loadUnlocked() map[string]Bookmark {
	if _, err := os.Stat(m.filePath); os.IsNotExist(err) {
		return make(map[string]Bookmark)
	}

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return make(map[string]Bookmark)
	}

	var bookmarks map[string]Bookmark
	if err := json.Unmarshal(data, &bookmarks); err != nil {
		return make(map[string]Bookmark)
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
