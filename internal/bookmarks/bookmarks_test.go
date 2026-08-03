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

func TestBookmarks_SaveHistoryResult(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "saved_paths.json")
	mgr := NewManager(tempFile)

	bms, _ := mgr.SaveBookmark("/projects/my-app", "My App")
	var savedID string
	for id := range bms {
		savedID = id
	}

	lr := LastResult{
		FileName:    "docs_my_app.md",
		TotalFiles:  42,
		TotalLines:  1200,
		TotalTokens: 45000,
		TokenMode:   "exact",
		SizeBytes:   98000,
		Elapsed:     1.2,
		GeneratedAt: "2026-08-03 00:00:00",
	}

	// Test SaveHistoryResult matching path
	updated, err := mgr.SaveHistoryResult("/projects/my-app", lr)
	if err != nil {
		t.Fatalf("SaveHistoryResult failed: %v", err)
	}

	if updated[savedID].LastResult == nil || updated[savedID].LastResult.TotalFiles != 42 {
		t.Errorf("expected LastResult TotalFiles == 42, got %+v", updated[savedID].LastResult)
	}

	// Test SaveHistoryResult non-matching path (does not error, does not modify)
	nonMatch, _ := mgr.SaveHistoryResult("/projects/other-app", lr)
	if nonMatch[savedID].LastResult.TotalFiles != 42 {
		t.Errorf("unmatched path affected bookmark result")
	}
}

func TestBookmarks_HistoryTimelineAndImportExport(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "saved_paths.json")
	mgr := NewManager(tempFile)

	bms, _ := mgr.SaveBookmark("/projects/my-app", "My App")
	var savedID string
	for id := range bms {
		savedID = id
	}

	res1 := LastResult{FileName: "res1.md", TotalFiles: 10, GeneratedAt: "2026-08-03 01:00:00"}
	res2 := LastResult{FileName: "res2.md", TotalFiles: 20, GeneratedAt: "2026-08-03 02:00:00"}

	// 1. Test SaveHistoryResult Appending
	mgr.SaveHistoryResult("/projects/my-app", res1)
	updated, _ := mgr.SaveHistoryResult("/projects/my-app", res2)

	bm := updated[savedID]
	if len(bm.History) != 2 {
		t.Fatalf("expected 2 history items, got %d", len(bm.History))
	}
	// Verify descending order by timestamp (res2 is newest)
	if bm.History[0].FileName != "res2.md" {
		t.Errorf("expected newest scan 'res2.md' at index 0, got %q", bm.History[0].FileName)
	}

	// 2. Test DeleteHistoryItem
	afterDelete, err := mgr.DeleteHistoryItem(savedID, "2026-08-03 01:00:00")
	if err != nil {
		t.Fatalf("DeleteHistoryItem failed: %v", err)
	}
	if len(afterDelete[savedID].History) != 1 {
		t.Errorf("expected 1 history item after selective delete, got %d", len(afterDelete[savedID].History))
	}

	// 3. Test Export & Import with Merge & Timestamp Overwrite
	exportBytes, err := mgr.ExportData()
	if err != nil {
		t.Fatalf("ExportData failed: %v", err)
	}

	tempFile2 := filepath.Join(t.TempDir(), "saved_paths2.json")
	mgr2 := NewManager(tempFile2)

	// Create local bookmark with same path and res1
	mgr2.SaveBookmark("/projects/my-app", "My App Local")
	mgr2.SaveHistoryResult("/projects/my-app", res1)

	// Import exported backup from mgr (contains res2)
	importedBms, err := mgr2.ImportData(exportBytes)
	if err != nil {
		t.Fatalf("ImportData failed: %v", err)
	}

	// Should merge history items res1 and res2
	var impID string
	for id := range importedBms {
		impID = id
	}

	impBm := importedBms[impID]
	if len(impBm.History) != 2 {
		t.Errorf("expected 2 history items after cumulative import merge, got %d", len(impBm.History))
	}

	// 4. Test ClearHistory
	cleared, err := mgr2.ClearHistory(impID)
	if err != nil {
		t.Fatalf("ClearHistory failed: %v", err)
	}
	if len(cleared[impID].History) != 0 || cleared[impID].LastResult != nil {
		t.Errorf("expected empty history after ClearHistory")
	}
}
