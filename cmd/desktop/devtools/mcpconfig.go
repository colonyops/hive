package main

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

type mcpConfig struct {
	Servers map[string]mcpServerEntry `json:"mcpServers"`
}

type mcpServerEntry struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// writeMCPConfig renders the project-scoped MCP config Claude Code reads from
// the worktree root, so an agent started here reaches the dev app's servers
// without a per-user registration. The blank onboarding instance writes none:
// two instances cannot share the one file, and the regular instance is the
// dev loop.
func (d *devtools) writeMCPConfig(env map[string]string) error {
	if d.mcpPath == "" {
		return nil
	}
	httpPort := env[settings.EnvHTTPPort]
	mcpPort := env["WAILS_MCP_PORT"]
	if httpPort == "" || mcpPort == "" {
		return fmt.Errorf("render %s: launch env carries no ports", d.mcpPath)
	}
	mcpHost := env["WAILS_MCP_HOST"]
	if mcpHost == "" {
		mcpHost = "127.0.0.1"
	}
	loopback := func(host, port, path string) string {
		return "http://" + net.JoinHostPort(host, port) + path
	}
	config := mcpConfig{Servers: map[string]mcpServerEntry{
		"hive-desktop":    {Type: "http", URL: loopback("127.0.0.1", httpPort, "/mcp")},
		"hive-canvas":     {Type: "http", URL: loopback("127.0.0.1", httpPort, "/mcp/canvas")},
		"hive-desktop-ui": {Type: "http", URL: loopback(mcpHost, mcpPort, "/mcp")},
	}}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(d.mcpPath, append(data, '\n'), ".mcp-*.json")
}
