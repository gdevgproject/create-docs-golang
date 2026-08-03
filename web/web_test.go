package web

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestFrontend_AppJsSyntax verifies that web/app.js is free of JavaScript syntax errors using node -c
func TestFrontend_AppJsSyntax(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node executable not found on system, skipping JS syntax check")
	}

	cmd := exec.Command(nodePath, "-c", "app.js")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("web/app.js syntax check failed: %v\nStderr: %s", err, stderr.String())
	}
}

// TestFrontend_RequiredDOMElementIDs verifies that web/index.html contains all critical DOM element IDs required by web/app.js
func TestFrontend_RequiredDOMElementIDs(t *testing.T) {
	htmlBytes, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	htmlContent := string(htmlBytes)

	requiredIDs := []string{
		"path-input",
		"btn-add-bm",
		"bm-note",
		"bm-list",
		"bm-count",
		"history-panel",
		"hp-bm-name",
		"hp-timeline",
		"btn-hp-clear",
		"ver-info",
		"btn-manual-check",
		"btn-check-update",
		"update-card",
		"update-card-title",
		"update-card-pct",
		"update-progress-fill",
		"update-card-sub",
		"btn-restart-now",
		"stats-cards",
		"stat-files",
		"stat-lines",
		"stat-tokens",
		"stat-size",
		"stat-time",
		"stat-date",
		"btn-gen",
		"btn-copy",
		"btn-download",
		"btn-load",
		"editor",
		"status-panel",
		"percent-text",
		"progress-fill",
		"log-text",
		"btn-export-bm",
		"btn-import-bm",
		"import-file-input",
	}

	for _, id := range requiredIDs {
		expectedAttr := `id="` + id + `"`
		if !strings.Contains(htmlContent, expectedAttr) {
			t.Errorf("web/index.html is missing required DOM element ID: %q", id)
		}
	}
}

// TestFrontend_AsyncSSEEventListeners verifies that event listeners containing await calls in web/app.js use async callbacks
func TestFrontend_AsyncSSEEventListeners(t *testing.T) {
	jsBytes, err := os.ReadFile("app.js")
	if err != nil {
		t.Fatalf("failed to read app.js: %v", err)
	}
	jsContent := string(jsBytes)

	// Verify complete event listener uses async e => or async (e) =>
	if strings.Contains(jsContent, "addEventListener('complete', e =>") || strings.Contains(jsContent, "addEventListener('complete', (e) =>") {
		t.Errorf("web/app.js contains synchronous addEventListener('complete', ...) callback which causes SyntaxError when calling await!")
	}

	if !strings.Contains(jsContent, "addEventListener('complete', async e =>") && !strings.Contains(jsContent, "addEventListener('complete', async (e) =>") {
		t.Errorf("web/app.js does not contain expected async addEventListener('complete', async ...) handler!")
	}
}

// TestFrontend_NoLegacyModalReferences verifies that app.js does not contain dead references to legacy history modal
func TestFrontend_NoLegacyModalReferences(t *testing.T) {
	jsBytes, err := os.ReadFile("app.js")
	if err != nil {
		t.Fatalf("failed to read app.js: %v", err)
	}
	jsContent := string(jsBytes)

	if strings.Contains(jsContent, "historyModal:") || strings.Contains(jsContent, "showHistoryModal") {
		t.Errorf("web/app.js still contains legacy modal references after migrating to persistent right history panel!")
	}
}
