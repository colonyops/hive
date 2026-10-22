package app

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodeMCPArgsKeepInstancesSeparate(t *testing.T) {
	for _, base := range []string{"http://127.0.0.1:50505", "http://127.0.0.1:56072"} {
		args, err := codeMCPArgs(base)
		require.NoError(t, err)
		var config struct {
			Servers map[string]struct {
				URL string `json:"url"`
			} `json:"mcpServers"`
		}
		require.NoError(t, json.Unmarshal([]byte(args["claude"][1]), &config))
		require.Equal(t, base+"/mcp/drive", config.Servers["hive-drive"].URL)
		require.Equal(t, base+"/mcp", config.Servers["hive-desktop"].URL)
		require.Equal(t, []string{"-c", `mcp_servers.hive-drive.url="` + base + `/mcp/drive"`, "-c", `mcp_servers.hive-desktop.url="` + base + `/mcp"`}, args["codex"])
		require.NotContains(t, args, "custom-wrapper")
	}
	args, err := codeMCPArgs("")
	require.NoError(t, err)
	require.Empty(t, args)
}
