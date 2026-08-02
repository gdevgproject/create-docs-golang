package updater

import (
	"os"
	"testing"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		current  string
		latest   string
		expected bool
	}{
		{"v1.0.0", "v1.0.1", true},
		{"v1.0.0", "v1.1.0", true},
		{"v1.0.0", "v2.0.0", true},
		{"v1.4.0", "v1.5.2", true},
		{"1.0.0", "1.0.0", false},
		{"v1.1.0", "v1.0.0", false},
		{"v1.0.1", "v1.0.0", false},
		{"v1.5.2", "v1.5.2", false},
	}

	for _, tt := range tests {
		got := IsNewerVersion(tt.current, tt.latest)
		if got != tt.expected {
			t.Errorf("IsNewerVersion(%q, %q) = %v; want %v", tt.current, tt.latest, got, tt.expected)
		}
	}
}

func TestGetProgress(t *testing.T) {
	testProg := DownloadProgress{
		State:           "downloading",
		Percent:         45,
		DownloadedBytes: 4500,
		TotalBytes:      10000,
	}

	setProgress(testProg)

	got := GetProgress()
	if got.State != "downloading" || got.Percent != 45 || got.DownloadedBytes != 4500 {
		t.Errorf("setProgress / GetProgress mismatch. Got: %+v, Want: %+v", got, testProg)
	}

	// Reset state for isolation
	setProgress(DownloadProgress{State: "idle"})
}

func TestCleanupOldFiles(t *testing.T) {
	execPath, err := os.Executable()
	if err != nil {
		t.Skip("skipping os.Executable test")
	}

	oldFile := execPath + ".old"
	tmpFile := execPath + ".tmp"

	_ = os.WriteFile(oldFile, []byte("old"), 0644)
	_ = os.WriteFile(tmpFile, []byte("tmp"), 0644)

	CleanupOldFiles()

	if _, err := os.Stat(oldFile); err == nil {
		t.Errorf("CleanupOldFiles failed to remove .old file")
	}
	if _, err := os.Stat(tmpFile); err == nil {
		t.Errorf("CleanupOldFiles failed to remove .tmp file")
	}
}
