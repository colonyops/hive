package agentws

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
)

const mcpJSONFileName = ".mcp.json"

// MCPWiring is one generated MCP config file: what the generator writes, and
// whether a CLI reading it is confined to exactly that set.
//
// It no longer carries launch flags. A workspace's command template names the
// file itself (LaunchData.MCPConfig), so wiring is purely about what lands on
// disk — and the generator writes every wiring into every workspace, which is
// what lets a template point an unknown CLI at a format it happens to read.
type MCPWiring struct {
	// File is written by the generator, relative to the workspace directory.
	File string
	// Render encodes the resolved servers in that file's format.
	Render func(servers map[string]mcpcatalog.Server) ([]byte, error)
	// Bounded reports whether a CLI loading this file is confined to exactly
	// this set. When false the agent also loads its own global servers, and
	// the UI says so — a tool set the user cannot see is not one they agreed
	// to.
	Bounded bool
}

// agentWirings maps an agent label to the MCP config format it reads. An
// agent absent here is not an error: its workspace still generates every
// file, and its template decides which (if any) to name.
var agentWirings = map[string]MCPWiring{
	"claude": {
		File:    mcpJSONFileName,
		Render:  renderMCPJSON,
		Bounded: true,
	},
	"codex": {
		File:    ".codex/config.toml",
		Render:  renderCodexTOML,
		Bounded: false,
	},
}

// conversationProbes report whether an agent persisted a conversation under a
// session id. Only claude has one; every other agent resumes unconditionally
// and reports its own state in the pane.
var conversationProbes = map[string]func(id string) bool{
	"claude": claudeConversationExists,
}

// MCPBounded reports whether agent's MCP config confines it to exactly the
// workspace's declared servers. ok is false for an agent with no known wiring,
// which the caller (building a user-facing notice) treats the same as
// "nothing to bound."
func MCPBounded(agent string) (bounded, ok bool) {
	wiring, exists := agentWirings[agent]
	if !exists {
		return false, false
	}
	return wiring.Bounded, true
}

// HasConversation reports whether agent is known to hold a persisted
// conversation under id — true when Hive cannot tell, so an uncertain caller
// resumes and lets the agent report its own state.
func HasConversation(agent, id string) bool {
	probe, ok := conversationProbes[agent]
	if !ok {
		return true
	}
	return probe(id)
}

// claudeConversationExists reports whether claude persisted a conversation
// under id. claude writes <config>/projects/<munged-cwd>/<session-id>.jsonl on
// the first message of a conversation — never at launch — with config being
// ~/.claude or $CLAUDE_CONFIG_DIR. The id is a uuid Hive minted, so matching
// the bare filename across every project directory is collision-safe and
// independent of claude's cwd-munging scheme.
//
// The check fails toward resuming: absence is only declared over a readable
// projects tree, because a wrong "absent" silently abandons a real
// conversation, while a wrong "present" merely reproduces the resume error
// the caller would have hit anyway.
func claudeConversationExists(id string) bool {
	root := os.Getenv("CLAUDE_CONFIG_DIR")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return true
		}
		root = filepath.Join(home, ".claude")
	}
	projects := filepath.Join(root, "projects")
	if info, err := os.Stat(projects); err != nil || !info.IsDir() {
		return true
	}
	matches, err := filepath.Glob(filepath.Join(projects, "*", id+".jsonl"))
	if err != nil {
		return true
	}
	return len(matches) > 0
}

// mcpJSONServer is one entry under .mcp.json's "mcpServers" — claude's own
// format. A remote server carries "type" plus url/headers; a stdio server
// carries command/args/env and no "type" (claude infers stdio from the
// absence of one).
type mcpJSONServer struct {
	Type    string            `json:"type,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// renderMCPJSON encodes servers as claude's .mcp.json. encoding/json sorts a
// map's string keys on encode, which is what gives both the server id order
// and the id-to-entry order their determinism — nothing here ranges a map
// straight into output.
func renderMCPJSON(servers map[string]mcpcatalog.Server) ([]byte, error) {
	out := struct {
		MCPServers map[string]mcpJSONServer `json:"mcpServers"`
	}{MCPServers: make(map[string]mcpJSONServer, len(servers))}

	for id, s := range servers {
		entry := mcpJSONServer{
			Command: s.Command,
			Args:    s.Args,
			Env:     s.Env,
			URL:     s.URL,
			Headers: s.Headers,
		}
		if s.BearerTokenEnv != "" {
			entry.Headers = maps.Clone(s.Headers)
			if entry.Headers == nil {
				entry.Headers = map[string]string{}
			}
			// The empty default keeps the server loadable outside Hive.
			entry.Headers["Authorization"] = "Bearer ${" + s.BearerTokenEnv + ":-}"
		}
		if s.Transport == mcpcatalog.TransportHttp || s.Transport == mcpcatalog.TransportSse {
			entry.Type = string(s.Transport)
			entry.Command = ""
			entry.Args = nil
			entry.Env = nil
		}
		out.MCPServers[id] = entry
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("agentws: encode .mcp.json: %w", err)
	}
	return append(data, '\n'), nil
}

// renderCodexTOML hand-rolls codex's .codex/config.toml: one
// [mcp_servers.<id>] table per server, key-sorted by id, with basic-string
// escaping for backslashes, quotes and control characters. No TOML library
// exists in the module graph (verified — go.mod and go.sum carry none), and a
// full encoder for a subset this small would only trade explicit key
// ordering here for a marshaller's own habits.
func renderCodexTOML(servers map[string]mcpcatalog.Server) ([]byte, error) {
	ids := make([]string, 0, len(servers))
	for id := range servers {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var buf strings.Builder
	for i, id := range ids {
		if i > 0 {
			buf.WriteByte('\n')
		}
		s := servers[id]
		fmt.Fprintf(&buf, "[mcp_servers.%s]\n", tomlKey(id))
		if s.Command != "" {
			fmt.Fprintf(&buf, "command = %s\n", tomlString(s.Command))
		}
		if len(s.Args) > 0 {
			fmt.Fprintf(&buf, "args = %s\n", tomlStringArray(s.Args))
		}
		if len(s.Env) > 0 {
			fmt.Fprintf(&buf, "env = %s\n", tomlStringMap(s.Env))
		}
		if s.URL != "" {
			fmt.Fprintf(&buf, "url = %s\n", tomlString(s.URL))
		}
		if len(s.Headers) > 0 {
			fmt.Fprintf(&buf, "headers = %s\n", tomlStringMap(s.Headers))
		}
		if s.BearerTokenEnv != "" {
			fmt.Fprintf(&buf, "bearer_token_env_var = %s\n", tomlString(s.BearerTokenEnv))
		}
	}
	return []byte(buf.String()), nil
}

// tomlKey renders a TOML key, quoting it as a basic string unless every
// character is bare-key safe ([A-Za-z0-9_-]) — a server id or an env var name
// with any other character (a space, a dot) would otherwise produce invalid
// TOML or silently change the key's meaning (a bare "a.b" is two nested
// tables, not one key named "a.b").
func tomlKey(s string) string {
	if isBareKey(s) {
		return s
	}
	return tomlString(s)
}

func isBareKey(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

// tomlString renders s as a TOML basic string: quoted, with backslashes,
// quotes and control characters escaped. This is the writer's one
// correctness-sensitive piece — see TestRenderCodexTOMLEscapesReservedStrings.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// tomlStringArray renders a TOML array of strings. Order is preserved, never
// sorted — these are command-line arguments, and their order is semantic.
func tomlStringArray(vals []string) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = tomlString(v)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// tomlStringMap renders a TOML inline table of string values, key-sorted —
// unlike an argument list, Env and Headers are Go maps with no order of their
// own, so ranging them straight into output would be nondeterministic.
func tomlStringMap(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = tomlKey(k) + " = " + tomlString(m[k])
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}
