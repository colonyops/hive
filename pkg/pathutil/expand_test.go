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
			gotE, err := ExpandHomeE(tt.path)
			if err != nil {
				t.Fatalf("ExpandHomeE(%q): %v", tt.path, err)
			}
			if gotE != tt.want {
				t.Errorf("ExpandHomeE(%q) = %q, want %q", tt.path, gotE, tt.want)
			}
		})
	}
}

func TestExpandHomeWithoutAHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("home", "")

	if _, err := ExpandHomeE("~/src"); err == nil {
		t.Fatal("ExpandHomeE(~/src) with no home: want an error")
	}
	if got := ExpandHome("~/src"); got != "~/src" {
		t.Errorf("ExpandHome(~/src) with no home = %q, want the literal path", got)
	}
	if got, err := ExpandHomeE("/abs"); err != nil || got != "/abs" {
		t.Errorf("ExpandHomeE(/abs) = %q, %v; a path without ~ needs no home", got, err)
	}
}

func TestXDGHomes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	if got, want := XDGConfigHome(), filepath.Join(home, ".config"); got != want {
		t.Errorf("XDGConfigHome() = %q, want %q", got, want)
	}
	if got, want := XDGDataHome(), filepath.Join(home, ".local", "share"); got != want {
		t.Errorf("XDGDataHome() = %q, want %q", got, want)
	}

	t.Setenv("XDG_CONFIG_HOME", "/xdg/config")
	t.Setenv("XDG_DATA_HOME", "/xdg/data")
	if got := XDGConfigHome(); got != "/xdg/config" {
		t.Errorf("XDGConfigHome() = %q, want /xdg/config", got)
	}
	if got := XDGDataHome(); got != "/xdg/data" {
		t.Errorf("XDGDataHome() = %q, want /xdg/data", got)
	}
}
