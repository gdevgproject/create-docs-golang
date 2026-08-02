package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanner_ScanProjectFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create test folder structure across multiple ecosystems
	os.MkdirAll(filepath.Join(tempDir, "src"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "node_modules", "lib"), 0755)
	os.MkdirAll(filepath.Join(tempDir, ".next", "cache"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "target", "classes"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "bin", "Debug"), 0755)
	os.MkdirAll(filepath.Join(tempDir, ".git"), 0755)

	os.WriteFile(filepath.Join(tempDir, "src", "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	os.WriteFile(filepath.Join(tempDir, "src", "logo.png"), []byte("dummy binary"), 0644)
	os.WriteFile(filepath.Join(tempDir, ".env"), []byte("SECRET=123"), 0644)
	os.WriteFile(filepath.Join(tempDir, "node_modules", "lib", "index.js"), []byte("console.log()"), 0644)
	os.WriteFile(filepath.Join(tempDir, ".next", "cache", "build.js"), []byte("build"), 0644)
	os.WriteFile(filepath.Join(tempDir, "target", "classes", "App.class"), []byte("class"), 0644)

	s := NewScanner()

	files, err := s.ScanProjectFiles(tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 valid files (src/main.go and src/logo.png), got %d: %v", len(files), files)
	}

	for _, f := range files {
		if strings.Contains(f, "node_modules") || strings.Contains(f, ".git") || strings.Contains(f, ".env") || strings.Contains(f, ".next") || strings.Contains(f, "target") {
			t.Errorf("found excluded file/dir in results: %s", f)
		}
	}
}

func TestScanner_GitignoreRules(t *testing.T) {
	tempDir := t.TempDir()

	os.MkdirAll(filepath.Join(tempDir, "custom_build"), 0755)
	os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("custom_build\ntemp.tmp\nsecret.key\n# comment\n"), 0644)
	os.WriteFile(filepath.Join(tempDir, "app.js"), []byte("console.log('app');"), 0644)
	os.WriteFile(filepath.Join(tempDir, "secret.key"), []byte("12345"), 0644)
	os.WriteFile(filepath.Join(tempDir, "temp.tmp"), []byte("temp"), 0644)
	os.WriteFile(filepath.Join(tempDir, "custom_build", "out.js"), []byte("out"), 0644)

	s := NewScanner()

	files, err := s.ScanProjectFiles(tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, f := range files {
		base := filepath.Base(f)
		if base == "secret.key" || base == "temp.tmp" || strings.Contains(f, "custom_build") {
			t.Errorf("file %s should have been excluded by .gitignore", f)
		}
	}

	foundApp := false
	for _, f := range files {
		if filepath.Base(f) == "app.js" {
			foundApp = true
			break
		}
	}
	if !foundApp {
		t.Errorf("app.js should be included in scan results")
	}
}

func TestScanner_IsBinary(t *testing.T) {
	s := NewScanner()

	tests := []struct {
		ext      string
		expected bool
	}{
		{"png", true},
		{".PNG", true},
		{"jpg", true},
		{"go", false},
		{"js", false},
		{"pdf", true},
		{"exe", true},
		{"zip", true},
		{"jar", true},
		{"pyc", true},
	}

	for _, tt := range tests {
		got := s.IsBinary(tt.ext)
		if got != tt.expected {
			t.Errorf("IsBinary(%q) = %v; want %v", tt.ext, got, tt.expected)
		}
	}
}

func TestGenerateDirectoryTree(t *testing.T) {
	root := "/my/project"
	files := []string{
		"/my/project/src/main.go",
		"/my/project/src/utils/math.go",
		"/my/project/README.md",
	}

	tree := GenerateDirectoryTree(root, files)

	expectedSubstrings := []string{
		"project/",
		"├── README.md",
		"└── src",
		"    ├── main.go",
		"    └── utils",
		"        └── math.go",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(tree, sub) {
			t.Errorf("tree output missing expected substring %q. Output:\n%s", sub, tree)
		}
	}
}

func TestScanner_CountProjectLinesFast(t *testing.T) {
	tempDir := t.TempDir()

	file1 := filepath.Join(tempDir, "a.txt")
	file2 := filepath.Join(tempDir, "b.png")

	os.WriteFile(file1, []byte("line1\nline2\nline3"), 0644)
	os.WriteFile(file2, []byte("binary data"), 0644)

	s := NewScanner()
	lines := s.CountProjectLinesFast([]string{file1, file2}, 10485760)

	if lines != 3 {
		t.Errorf("expected 3 lines, got %d", lines)
	}
}
