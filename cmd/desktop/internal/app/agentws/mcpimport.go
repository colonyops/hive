package agentws

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseMCPImport parses a pasted MCP configuration into library servers. Both
// shapes in the wild are accepted: claude's {"mcpServers": {...}} wrapper and
// a bare {"<id>": {...}} map of servers. Unknown keys (autoApprove, disabled,
// and whatever else other tools add) are deliberately ignored rather than
// failing a paste of a real-world config. Every entry is validated with the
// same rules mcps.yaml is, so a paste cannot land a server the loader would
// then refuse.
func ParseMCPImport(data []byte) (map[string]MCPServer, error) {
	var wrapper struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	entries := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.MCPServers != nil {
		entries = wrapper.MCPServers
	} else if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("agentws: mcp import: not a JSON object of servers: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("agentws: mcp import: no servers found")
	}

	servers := make(map[string]MCPServer, len(entries))
	for id, raw := range entries {
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("agentws: mcp import: server id must not be empty")
		}
		var entry mcpJSONServer
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, fmt.Errorf("agentws: mcp import: server %q is not an object — wrap a single server as {%q: {...}}", id, "my-server")
		}
		srv := MCPServer{
			Type:    entry.Type,
			Command: entry.Command,
			Args:    entry.Args,
			Env:     entry.Env,
			URL:     entry.URL,
			Headers: entry.Headers,
		}
		if err := srv.Validate(); err != nil {
			return nil, fmt.Errorf("agentws: mcp import: server %q: %w", id, err)
		}
		servers[id] = srv
	}
	return servers, nil
}
