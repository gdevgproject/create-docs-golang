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

func TestBookmarks_UpdateNoteAndReorder(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "saved_paths.json")
	mgr := NewManager(tempFile)

	_, err := mgr.SaveBookmark("/path/1", "Proj 1")
	if err != nil {
		t.Fatalf("SaveBookmark failed: %v", err)
	}
	mgr.SaveBookmark("/path/2", "Proj 2")

	var id1, id2 string
	allBms, _ := mgr.GetBookmarks()
	for id, bm := range allBms {
		if bm.Path == "/path/1" {
			id1 = id
		}
		if bm.Path == "/path/2" {
			id2 = id
		}
	}

	// Test UpdateNote (Rename)
	updated, err := mgr.UpdateNote(id1, "Renamed Proj 1")
	if err != nil {
		t.Fatalf("UpdateNote failed: %v", err)
	}
	if updated[id1].Note != "Renamed Proj 1" {
		t.Errorf("expected updated note 'Renamed Proj 1', got %q", updated[id1].Note)
	}

	// Test Reorder
	reordered, err := mgr.ReorderBookmarks([]string{id2, id1})
	if err != nil {
		t.Fatalf("ReorderBookmarks failed: %v", err)
	}
	if reordered[id2].Order >= reordered[id1].Order {
		t.Errorf("expected id2 order (%d) < id1 order (%d)", reordered[id2].Order, reordered[id1].Order)
	}
}
