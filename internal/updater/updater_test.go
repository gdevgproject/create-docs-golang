package updater

import "testing"

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		current  string
		latest   string
		expected bool
	}{
		{"v1.0.0", "v1.0.1", true},
		{"v1.0.0", "v1.1.0", true},
		{"v1.0.0", "v2.0.0", true},
		{"1.0.0", "1.0.0", false},
		{"v1.1.0", "v1.0.0", false},
		{"v1.0.1", "v1.0.0", false},
	}

	for _, tt := range tests {
		got := IsNewerVersion(tt.current, tt.latest)
		if got != tt.expected {
			t.Errorf("IsNewerVersion(%q, %q) = %v; want %v", tt.current, tt.latest, got, tt.expected)
		}
	}
}
