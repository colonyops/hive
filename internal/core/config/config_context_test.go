package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContextDir_Default(t *testing.T) {
	cfg := &Config{DataDir: "/tmp/hive-data"}
	want := filepath.Join("/tmp/hive-data", "context")
	if got := cfg.ContextDir(); got != want {
		t.Errorf("ContextDir() = %q, want %q", got, want)
	}
}

func TestContextDir_BaseDir(t *testing.T) {
	cfg := &Config{
		DataDir: "/tmp/hive-data",
		Context: ContextConfig{BaseDir: "/custom/context"},
	}
	want := "/custom/context"
	if got := cfg.ContextDir(); got != want {
		t.Errorf("ContextDir() = %q, want %q", got, want)
	}
}

func TestContextDir_BaseDirTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home dir: %v", err)
	}

	cfg := &Config{
		DataDir: "/tmp/hive-data",
		Context: ContextConfig{BaseDir: "~/my-context"},
	}
	want := filepath.Join(home, "my-context")
	if got := cfg.ContextDir(); got != want {
		t.Errorf("ContextDir() = %q, want %q", got, want)
	}
}

func TestRepoContextDir_WithBaseDir(t *testing.T) {
	cfg := &Config{
		DataDir: "/tmp/hive-data",
		Context: ContextConfig{BaseDir: "/custom/context"},
	}
	want := filepath.Join("/custom/context", "myorg", "myrepo")
	if got := cfg.RepoContextDir("myorg", "myrepo"); got != want {
		t.Errorf("RepoContextDir() = %q, want %q", got, want)
	}
}

func TestSharedContextDir_WithBaseDir(t *testing.T) {
	cfg := &Config{
		DataDir: "/tmp/hive-data",
		Context: ContextConfig{BaseDir: "/custom/context"},
	}
	want := filepath.Join("/custom/context", "shared")
	if got := cfg.SharedContextDir(); got != want {
		t.Errorf("SharedContextDir() = %q, want %q", got, want)
	}
}
