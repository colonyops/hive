package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestValidateContextBaseDir(t *testing.T) {
	tests := []struct {
		name    string
		baseDir string
		setup   func(t *testing.T) string // returns dir to use, if needed
		wantErr bool
	}{
		{name: "empty is valid", baseDir: "", wantErr: false},
		{name: "absolute path exists", setup: func(t *testing.T) string { return t.TempDir() }, wantErr: false},
		{name: "absolute path not exists is valid", baseDir: "/tmp/nonexistent-hive-test-dir-12345", wantErr: false},
		{name: "tilde path is valid", baseDir: "~/some-dir", wantErr: false},
		{name: "relative path rejected", baseDir: "relative/path", wantErr: true},
		{name: "tilde username rejected", baseDir: "~bob/data", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.DataDir = t.TempDir()
			if tt.setup != nil {
				cfg.Context.BaseDir = tt.setup(t)
			} else {
				cfg.Context.BaseDir = tt.baseDir
			}
			err := cfg.validateContextBaseDir()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "context.base_dir")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
