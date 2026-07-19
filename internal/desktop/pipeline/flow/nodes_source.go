package flow

import (
	"fmt"
	"strings"
)

// githubSourceKindPrefix is how profiles source kinds name the github
// family (e.g. "github-search", "github-notifications"); a github-source
// node may reference any source whose kind carries this prefix.
const githubSourceKindPrefix = "github-"

// rpcSourceKind is the profiles source kind an rpc-source node may reference.
const rpcSourceKind = "rpc"

// GithubSourceConfig is a github-source node: 0 inputs, 1 output. It emits
// the items produced by the referenced profiles source, which must be one
// of the github-* kinds.
type GithubSourceConfig struct {
	Source string `yaml:"source"`
}

func (c *GithubSourceConfig) Inputs() int  { return 0 }
func (c *GithubSourceConfig) Outputs() int { return 1 }

func (c *GithubSourceConfig) Validate(refs Refs) error {
	if c.Source == "" {
		return fmt.Errorf("github-source: source is required")
	}
	if !validSlug(c.Source) {
		return fmt.Errorf("github-source: source %q is not a valid id", c.Source)
	}
	kind, ok := refsResolveSource(refs, c.Source)
	if !ok {
		return fmt.Errorf("github-source: source %q: unresolved reference", c.Source)
	}
	if !strings.HasPrefix(kind, githubSourceKindPrefix) {
		return fmt.Errorf("github-source: source %q has kind %q, want a github-* source", c.Source, kind)
	}
	return nil
}

// RPCSourceConfig is an rpc-source node: 0 inputs, 1 output. It emits the
// items produced by the referenced profiles source, which must be of kind
// "rpc".
type RPCSourceConfig struct {
	Source string `yaml:"source"`
}

func (c *RPCSourceConfig) Inputs() int  { return 0 }
func (c *RPCSourceConfig) Outputs() int { return 1 }

func (c *RPCSourceConfig) Validate(refs Refs) error {
	if c.Source == "" {
		return fmt.Errorf("rpc-source: source is required")
	}
	if !validSlug(c.Source) {
		return fmt.Errorf("rpc-source: source %q is not a valid id", c.Source)
	}
	kind, ok := refsResolveSource(refs, c.Source)
	if !ok {
		return fmt.Errorf("rpc-source: source %q: unresolved reference", c.Source)
	}
	if kind != rpcSourceKind {
		return fmt.Errorf("rpc-source: source %q has kind %q, want kind %q", c.Source, kind, rpcSourceKind)
	}
	return nil
}
