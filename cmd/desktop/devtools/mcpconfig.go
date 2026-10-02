package main

import (
	"encoding/json"
	"fmt"
	"net"
)

type mcpConfig struct {
	Servers map[string]mcpServerEntry `json:"mcpServers"`
}

type mcpServerEntry struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// writeMCPConfig renders the project-scoped MCP config Claude Code reads from
// the worktree root, so an agent started here reaches the dev app's Wails MCP
// server without a per-user registration. The blank onboarding instance
// writes none: two instances cannot share the one file, and the regular
// instance is the dev loop.
func (d *devtools) writeMCPConfig(env map[string]string) error {
	if d.mcpPath == "" {
		return nil
	}
	port := env["WAILS_MCP_PORT"]
	if port == "" {
		return fmt.Errorf("render %s: launch env carries no WAILS_MCP_PORT", d.mcpPath)
	}
	host := env["WAILS_MCP_HOST"]
	if host == "" {
		host = "127.0.0.1"
	}
	config := mcpConfig{Servers: map[string]mcpServerEntry{
		"hive-desktop-ui": {Type: "http", URL: "http://" + net.JoinHostPort(host, port) + "/mcp"},
	}}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(d.mcpPath, append(data, '\n'), ".mcp-*.json")
}
