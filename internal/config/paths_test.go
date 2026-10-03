package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDataDir(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "neither set uses the fallback", env: map[string]string{}, want: "/fallback"},
		{name: "HIVE_DATA_DIR", env: map[string]string{EnvDataDir: "/cli"}, want: "/cli"},
		{name: "desktop variable alone", env: map[string]string{EnvDesktopDataDir: "/desktop"}, want: "/desktop"},
		{name: "desktop variable wins over HIVE_DATA_DIR", env: map[string]string{EnvDesktopDataDir: "/desktop", EnvDataDir: "/cli"}, want: "/desktop"},
		{name: "empty values are unset", env: map[string]string{EnvDesktopDataDir: "", EnvDataDir: ""}, want: "/fallback"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(name string) string { return tt.env[name] }
			assert.Equal(t, tt.want, ResolveDataDir(getenv, "/fallback"))
		})
	}
}

func TestConfigPathIn(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, filepath.Join(dir, "config.yaml"), ConfigPathIn(dir), "no file names the path a writer creates")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "hive.yml"), nil, 0o644))
	assert.Equal(t, filepath.Join(dir, "hive.yml"), ConfigPathIn(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yml"), nil, 0o644))
	assert.Equal(t, filepath.Join(dir, "config.yml"), ConfigPathIn(dir), "probe order decides")
}
