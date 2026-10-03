package scripts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureExtracted(t *testing.T) {
	dataDir := t.TempDir()

	// First extraction should write files
	if err := EnsureExtracted(dataDir, "v1.0.0"); err != nil {
		t.Fatalf("first extraction: %v", err)
	}

	binDir := BinDir(dataDir)

	// Verify scripts exist and are executable
	for _, name := range scriptNames {
		path := filepath.Join(binDir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("script %s not found: %v", name, err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("script %s is not executable: %v", name, info.Mode())
		}
		if info.Size() == 0 {
			t.Errorf("script %s is empty", name)
		}
	}

	// Verify version marker
	marker, err := os.ReadFile(filepath.Join(binDir, ".version"))
	if err != nil {
		t.Fatalf("read version marker: %v", err)
	}
	if string(marker) != "v1.0.0" {
		t.Errorf("version marker = %q, want %q", string(marker), "v1.0.0")
	}
}

func TestEnsureExtracted_SkipsWhenVersionMatches(t *testing.T) {
	dataDir := t.TempDir()

	if err := EnsureExtracted(dataDir, "v1.0.0"); err != nil {
		t.Fatalf("first extraction: %v", err)
	}

	// Modify a script to detect if re-extraction happens
	sentinel := filepath.Join(BinDir(dataDir), "hive-tmux")
	if err := os.WriteFile(sentinel, []byte("modified"), 0o755); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	// Same version should skip
	if err := EnsureExtracted(dataDir, "v1.0.0"); err != nil {
		t.Fatalf("second extraction: %v", err)
	}

	data, _ := os.ReadFile(sentinel)
	if string(data) != "modified" {
		t.Error("same version should not re-extract")
	}
}

func TestEnsureExtracted_ReExtractsOnVersionChange(t *testing.T) {
	dataDir := t.TempDir()

	if err := EnsureExtracted(dataDir, "v1.0.0"); err != nil {
		t.Fatalf("first extraction: %v", err)
	}

	// Modify a script
	sentinel := filepath.Join(BinDir(dataDir), "hive-tmux")
	if err := os.WriteFile(sentinel, []byte("modified"), 0o755); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	// Different version should re-extract
	if err := EnsureExtracted(dataDir, "v2.0.0"); err != nil {
		t.Fatalf("re-extraction: %v", err)
	}

	data, _ := os.ReadFile(sentinel)
	if string(data) == "modified" {
		t.Error("new version should re-extract scripts")
	}
}

func TestScriptPaths(t *testing.T) {
	paths := ScriptPaths("/data")
	if got := paths["hive-tmux"]; got != "/data/bin/hive-tmux" {
		t.Errorf("hive-tmux path = %q, want %q", got, "/data/bin/hive-tmux")
	}
	if got := paths["agent-send"]; got != "/data/bin/agent-send" {
		t.Errorf("agent-send path = %q, want %q", got, "/data/bin/agent-send")
	}
}

func TestBinDir(t *testing.T) {
	if got := BinDir("/data"); got != "/data/bin" {
		t.Errorf("BinDir = %q, want %q", got, "/data/bin")
	}
}
