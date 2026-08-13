package web

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrontendJavaScriptSyntax(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node executable not found")
	}
	files := []string{"app.js"}
	err = filepath.WalkDir("js", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && filepath.Ext(path) == ".js" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		t.Run(filepath.ToSlash(file), func(t *testing.T) {
			command := exec.Command(nodePath, "--check", file)
			var stderr bytes.Buffer
			command.Stderr = &stderr
			if err := command.Run(); err != nil {
				t.Fatalf("%s syntax check failed: %v\n%s", file, err, stderr.String())
			}
		})
	}
}

func TestFrontendRequiredDOMElementIDs(t *testing.T) {
	htmlBytes, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlBytes)
	requiredIDs := []string{
		"path-input", "btn-add-bm", "bm-note", "bm-list", "bm-count",
		"projects-panel", "history-panel", "hp-bm-name", "hp-timeline",
		"btn-hp-clear", "ver-info", "btn-manual-check", "btn-check-update",
		"update-card", "update-card-title", "update-card-pct",
		"update-progress-fill", "update-card-sub", "btn-download-update",
		"btn-release-notes", "btn-restart-now", "stats-cards", "stat-files",
		"stat-lines", "stat-tokens", "stat-size", "stat-time", "stat-date",
		"btn-gen", "btn-cancel-gen", "btn-copy", "btn-download", "btn-load",
		"editor", "empty-state", "status-panel", "percent-text",
		"progress-fill", "log-text", "btn-export-bm", "btn-import-bm",
		"import-file-input", "btn-toggle-projects", "btn-toggle-history",
		"modal-dialog", "modal-content", "toast",
	}
	for _, id := range requiredIDs {
		if !strings.Contains(html, `id="`+id+`"`) {
			t.Errorf("index.html is missing id %q", id)
		}
	}
}

func TestFrontendGenerationCompletionHandlerIsAsync(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("js", "generator.js"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(content)
	if !strings.Contains(source, "addEventListener('complete', async (event) =>") {
		t.Error("generation completion listener must remain async")
	}
}

func TestFrontendHasNoHeartbeatOrKnownMojibake(t *testing.T) {
	files := []string{"index.html", "style.css", "app.js"}
	moduleFiles, err := filepath.Glob(filepath.Join("js", "*.js"))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, moduleFiles...)
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		source := string(content)
		if strings.Contains(source, "setInterval(sendPing") || strings.Contains(source, "function sendPing") {
			t.Errorf("%s contains the obsolete heartbeat", file)
		}
		for _, marker := range []string{"Ã", "Â", "â€”", "ðŸ"} {
			if strings.Contains(source, marker) {
				t.Errorf("%s contains mojibake marker %q", file, marker)
			}
		}
	}
}

func TestFrontendModulesAreEmbedded(t *testing.T) {
	embedded := GetFS()
	for _, path := range []string{
		"app.js",
		"js/core.js",
		"js/bookmarks.js",
		"js/generator.js",
		"js/tools.js",
		"js/updater.js",
	} {
		file, err := embedded.Open(path)
		if err != nil {
			t.Errorf("embedded frontend is missing %s: %v", path, err)
			continue
		}
		_ = file.Close()
	}
}

func TestFrontendResponsiveBreakpoints(t *testing.T) {
	content, err := os.ReadFile("style.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(content)
	for _, breakpoint := range []string{"@media (max-width: 1280px)", "@media (max-width: 920px)", "@media (max-width: 760px)"} {
		if !strings.Contains(css, breakpoint) {
			t.Errorf("style.css is missing responsive breakpoint %s", breakpoint)
		}
	}
}
