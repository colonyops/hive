package app

import (
	"context"
	"encoding/json"
	"strconv"
)

func (a *App) codeAgentArgs(ctx context.Context) map[string][]string {
	args, err := codeMCPArgs(a.MCPBaseURL(ctx))
	if err != nil {
		a.logger.Warn().Ctx(ctx).Err(err).Msg("could not configure instance MCP endpoints")
	}
	return args
}

func codeMCPArgs(base string) (map[string][]string, error) {
	if base == "" {
		return nil, nil
	}
	servers := map[string]map[string]string{
		"hive-drive":   {"type": "http", "url": base + "/mcp/drive"},
		"hive-desktop": {"type": "http", "url": base + "/mcp"},
	}
	encoded, err := json.Marshal(map[string]any{"mcpServers": servers})
	if err != nil {
		return nil, err
	}
	return map[string][]string{
		"claude": {"--mcp-config", string(encoded)},
		"codex": {
			"-c", "mcp_servers.hive-drive.url=" + strconv.Quote(base+"/mcp/drive"),
			"-c", "mcp_servers.hive-desktop.url=" + strconv.Quote(base+"/mcp"),
		},
	}, nil
}
