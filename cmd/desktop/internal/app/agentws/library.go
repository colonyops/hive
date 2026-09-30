package agentws

import (
	"fmt"

	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
)

// Library is the user's own MCP server library (mcps.yaml). The field set is
// what .mcp.json can express, because that is the format the generator emits
// — a library that could not describe a remote server would make the escape
// hatch narrower than the shipped catalogue.
type Library struct {
	Version int                  `yaml:"version"`
	Servers map[string]MCPServer `yaml:"servers,omitempty"`
}

// MCPServer is one user-declared server, keyed by id in Library.Servers. Type
// defaults to "stdio" when empty — both here and in Catalogue, since a
// Library value built directly (rather than through parseLibrary) must behave
// the same way.
type MCPServer struct {
	Title   string            `yaml:"title,omitempty"`
	Type    string            `yaml:"type,omitempty"`
	Command string            `yaml:"command,omitempty"`
	Args    []string          `yaml:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	URL     string            `yaml:"url,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

// Validate checks every server's id and fields.
func (l Library) Validate() error {
	for id, srv := range l.Servers {
		if id == "" {
			return fmt.Errorf("mcps.yaml: server id must not be empty")
		}
		if err := srv.Validate(); err != nil {
			return fmt.Errorf("mcps.yaml: server %q: %w", id, err)
		}
	}
	return nil
}

// effectiveType returns Type, defaulting an empty value to stdio.
func (s MCPServer) effectiveType() mcpcatalog.Transport {
	if s.Type == "" {
		return mcpcatalog.TransportStdio
	}
	return mcpcatalog.Transport(s.Type)
}

// Validate checks the field set against what its (defaulted) transport can
// express: stdio requires Command and rejects URL/Headers, http/sse require
// URL and reject Command/Args/Env.
func (s MCPServer) Validate() error {
	transport := s.effectiveType()
	switch transport {
	case mcpcatalog.TransportStdio:
		if s.Command == "" {
			return fmt.Errorf("stdio requires command")
		}
		if s.URL != "" || len(s.Headers) > 0 {
			return fmt.Errorf("stdio does not accept url or headers")
		}
	case mcpcatalog.TransportHttp, mcpcatalog.TransportSse:
		if s.URL == "" {
			return fmt.Errorf("%s requires url", transport)
		}
		if s.Command != "" || len(s.Args) > 0 || len(s.Env) > 0 {
			return fmt.Errorf("%s does not accept command, args or env", transport)
		}
	default:
		return fmt.Errorf("invalid type %q", s.Type)
	}
	return nil
}
