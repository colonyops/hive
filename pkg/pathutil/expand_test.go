package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home dir: %v", err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "bare tilde", path: "~", want: home},
		{name: "tilde with path", path: "~/Documents/work", want: filepath.Join(home, "Documents/work")},
		{name: "tilde username not expanded", path: "~username/path", want: "~username/path"},
		{name: "absolute path unchanged", path: "/usr/local/bin", want: "/usr/local/bin"},
		{name: "relative path unchanged", path: "relative/path", want: "relative/path"},
		{name: "empty string unchanged", path: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandHome(tt.path)
			if got != tt.want {
				t.Errorf("ExpandHome(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
