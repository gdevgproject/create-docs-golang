package bookmarks

import (
	"encoding/json"
	"os"
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

func TestBookmarks_SaveSamePathUpdatesExisting(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "saved_paths.json")
	manager := NewManager(tempFile)

	first, err := manager.SaveBookmark("D:/Projects/MyApp", "First name")
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.SaveBookmark("D:/Projects/MyApp", "Updated name")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("saving the same path created a duplicate: first=%d second=%d", len(first), len(second))
	}
	for id, bookmark := range second {
		if bookmark.Note != "Updated name" {
			t.Fatalf("bookmark note = %q, want Updated name", bookmark.Note)
		}
		if _, existed := first[id]; !existed {
			t.Fatal("updating a path must preserve the bookmark ID")
		}
	}
}

func TestBookmarks_RecoversValidBackup(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "saved_paths.json")
	manager := NewManager(tempFile)
	if _, err := manager.SaveBookmark("D:/Projects/One", "One"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SaveBookmark("D:/Projects/Two", "Two"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tempFile + ".bak"); err != nil {
		t.Fatalf("expected a backup generation: %v", err)
	}
	if err := os.WriteFile(tempFile, []byte("{corrupt"), 0600); err != nil {
		t.Fatal(err)
	}

	recovered, err := manager.GetBookmarks()
	if err != nil {
		t.Fatalf("valid backup was not recovered: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("recovered generation contains %d bookmarks, want 1", len(recovered))
	}
	data, err := os.ReadFile(tempFile)
	if err != nil || !json.Valid(data) {
		t.Fatalf("primary file was not repaired: err=%v data=%q", err, data)
	}
}

func TestBookmarks_CorruptionIsReportedWithoutDataReset(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "saved_paths.json")
	if err := os.WriteFile(tempFile, []byte("{} trailing"), 0600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(tempFile)
	if _, err := manager.GetBookmarks(); err == nil {
		t.Fatal("expected corrupt bookmark data to be reported")
	}
	if _, err := manager.SaveBookmark("D:/Projects/New", "New"); err == nil {
		t.Fatal("mutation must not overwrite corrupt bookmark data")
	}
	data, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{} trailing" {
		t.Fatalf("corrupt source was unexpectedly replaced: %q", data)
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

func TestBookmarks_LegacyMigrationAndCorruptedFile(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "saved_paths_legacy.json")

	// Write legacy JSON with last_result but no history array
	legacyJSON := `{
		"bm1": {
			"id": "bm1",
			"path": "D:/Projects/LegacyApp",
			"note": "Legacy App",
			"created_at": "2026-08-01 10:00:00",
			"last_result": {
				"file_name": "legacy_docs.md",
				"total": 15,
				"lines": 450,
				"tokens": 12000,
				"token_mode": "exact",
				"size": 35000,
				"elapsed": 0.8,
				"generated_at": "2026-08-01 10:05:00"
			}
		}
	}`

	if err := os.WriteFile(tempFile, []byte(legacyJSON), 0644); err != nil {
		t.Fatalf("failed to write legacy test file: %v", err)
	}

	mgr := NewManager(tempFile)
	bms, err := mgr.GetBookmarks()
	if err != nil {
		t.Fatalf("GetBookmarks failed on legacy JSON: %v", err)
	}

	bm1, exists := bms["bm1"]
	if !exists {
		t.Fatalf("expected bm1 to exist")
	}

	// Verify auto-migration populated History array from LastResult
	if len(bm1.History) != 1 {
		t.Fatalf("expected History slice of length 1 after legacy migration, got %d", len(bm1.History))
	}
	if bm1.History[0].FileName != "legacy_docs.md" {
		t.Errorf("expected migrated history filename 'legacy_docs.md', got %q", bm1.History[0].FileName)
	}

	// Test Import invalid JSON bytes
	_, err = mgr.ImportData([]byte("invalid json string"))
	if err == nil {
		t.Errorf("expected error on importing corrupted JSON bytes, got nil")
	}
}

func TestBookmarks_RenameHistoryItem(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "saved_paths_rename.json")
	mgr := NewManager(tempFile)

	bms, err := mgr.SaveBookmark("/projects/rename-app", "Rename App")
	if err != nil {
		t.Fatalf("SaveBookmark failed: %v", err)
	}

	var bmID string
	for id := range bms {
		bmID = id
	}

	res1 := LastResult{FileName: "res1.md", TotalFiles: 10, GeneratedAt: "2026-08-12 01:00:00"}
	res2 := LastResult{FileName: "res2.md", TotalFiles: 20, GeneratedAt: "2026-08-12 02:00:00"}

	mgr.SaveHistoryResult("/projects/rename-app", res1)
	mgr.SaveHistoryResult("/projects/rename-app", res2)

	// Test Renaming res2 (Latest scan)
	updated, err := mgr.RenameHistoryItem(bmID, "2026-08-12 02:00:00", "Sprint 1 Release")
	if err != nil {
		t.Fatalf("RenameHistoryItem failed: %v", err)
	}

	bm := updated[bmID]
	if bm.History[0].Label != "Sprint 1 Release" {
		t.Errorf("expected History[0] label 'Sprint 1 Release', got %q", bm.History[0].Label)
	}
	if bm.LastResult == nil || bm.LastResult.Label != "Sprint 1 Release" {
		t.Errorf("expected LastResult label to be synced to 'Sprint 1 Release', got %+v", bm.LastResult)
	}

	// Test Renaming res1 (Past scan)
	updated2, err := mgr.RenameHistoryItem(bmID, "2026-08-12 01:00:00", "Initial Baseline")
	if err != nil {
		t.Fatalf("RenameHistoryItem for past scan failed: %v", err)
	}
	if updated2[bmID].History[1].Label != "Initial Baseline" {
		t.Errorf("expected History[1] label 'Initial Baseline', got %q", updated2[bmID].History[1].Label)
	}

	// Test Error on non-existent timestamp
	_, err = mgr.RenameHistoryItem(bmID, "1999-01-01 00:00:00", "Invalid")
	if err == nil {
		t.Errorf("expected error when renaming non-existent timestamp, got nil")
	}

	// Test Export and Import preserves custom labels
	exportBytes, err := mgr.ExportData()
	if err != nil {
		t.Fatalf("ExportData failed: %v", err)
	}

	tempFile2 := filepath.Join(t.TempDir(), "saved_paths_imported.json")
	mgr2 := NewManager(tempFile2)
	imported, err := mgr2.ImportData(exportBytes)
	if err != nil {
		t.Fatalf("ImportData failed: %v", err)
	}

	var impID string
	for id := range imported {
		impID = id
	}
	impBm := imported[impID]
	if len(impBm.History) != 2 || impBm.History[0].Label != "Sprint 1 Release" || impBm.History[1].Label != "Initial Baseline" {
		t.Errorf("imported history labels mismatch: %+v", impBm.History)
	}
}
