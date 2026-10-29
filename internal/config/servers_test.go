package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadServers(t *testing.T) {
	t.Setenv("HIVE_DEFAULT_AGENT", "")
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`servers:
  - name: fixture
    ssh: fixture
    ssh_config: /state/ssh_config
    binary: /usr/local/bin/hive
`), 0o644))

	cfg, err := Load(path, t.TempDir())
	require.NoError(t, err)
	require.Len(t, cfg.Servers, 1)
	server := cfg.Servers[0]
	assert.Equal(t, "fixture", server.Name)
	assert.Equal(t, "/state/ssh_config", server.Host().ConfigFile)
	assert.Equal(t, "/usr/local/bin/hive", server.Host().Binary)
}

func TestValidateServers(t *testing.T) {
	tests := []struct {
		name    string
		servers []ServerConfig
		wantErr string
	}{
		{"valid", []ServerConfig{{Name: "a", SSH: "a"}, {Name: "b", SSH: "ada@b"}}, ""},
		{"duplicate", []ServerConfig{{Name: "a", SSH: "a"}, {Name: "a", SSH: "b"}}, "duplicate"},
		{"missing name", []ServerConfig{{SSH: "a"}}, "servers[0].name"},
		{"local is reserved", []ServerConfig{{Name: "local", SSH: "a"}}, "servers[0].name"},
		{"separator in name", []ServerConfig{{Name: "a:b", SSH: "a"}}, "servers[0].name"},
		{"option as host", []ServerConfig{{Name: "a", SSH: "-oProxyCommand=x"}}, "servers[0].ssh"},
		{"shell in binary", []ServerConfig{{Name: "a", SSH: "a", Binary: "hive;rm -rf ~"}}, "servers[0].ssh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&Config{Servers: tt.servers}).validateServers()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
