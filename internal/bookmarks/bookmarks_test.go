package bookmarks

import (
	"path/filepath"
	"testing"
)

func TestBookmarks_SaveGetDelete(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "saved_paths.json")
	mgr := NewManager(tempFile)

	// Test Get empty
	items, err := mgr.GetBookmarks()
	if err != nil {
		t.Fatalf("GetBookmarks failed: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 bookmarks, got %d", len(items))
	}

	// Test Save
	items, err = mgr.SaveBookmark("/var/www/project1", "Project 1")
	if err != nil {
		t.Fatalf("SaveBookmark failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(items))
	}

	var savedID string
	for id, bm := range items {
		savedID = id
		if bm.Note != "Project 1" {
			t.Errorf("expected Note 'Project 1', got %q", bm.Note)
		}
	}

	// Test Delete
	items, err = mgr.DeleteBookmark(savedID)
	if err != nil {
		t.Fatalf("DeleteBookmark failed: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 bookmarks after delete, got %d", len(items))
	}
}
