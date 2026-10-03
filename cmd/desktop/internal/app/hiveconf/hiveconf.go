// Package hiveconf reads and writes the external Hive CLI configuration shared
// with the `hive` binary. It reports raw declarations rather than Hive's merged
// defaults, so a fallback is never mistaken for a user choice (Bounded Context,
// architecture.md). An absent file is valid but not usable for repository
// sessions.
package hiveconf

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/agent"
	"github.com/colonyops/hive/pkg/pathutil"
	"gopkg.in/yaml.v3"
)

// AgentKind is the wire form of one agent.Known entry: what the profile is
// called in the config file, what to show a person, and the flags that turn
// off the agent's own permission prompts.
type AgentKind struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	// SkipPermissionFlags is empty for an agent with no known flag. Offering
	// the toggle for one of those would write a profile that does nothing.
	SkipPermissionFlags []string `json:"skipPermissionFlags"`
}

// AgentOption is an AgentKind plus whether this machine can actually run it.
type AgentOption struct {
	AgentKind
	// Installed reports whether the command resolved on the launch PATH. A
	// missing agent is still selectable: it may be installed later, and the
	// resolved PATH of a Dock launch is not always the one a person sees in
	// their terminal.
	Installed bool `json:"installed"`
}

// Profile is one agent profile as the config file declares it.
type Profile struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Flags   []string `json:"flags"`
}

// Workspace is one configured parent directory plus what is at that path now.
type Workspace struct {
	Path string `json:"path"`
	// Exists reports whether the directory is present. A configured path that
	// has been moved or is on an unmounted volume reads as missing rather
	// than as an error: the file is still valid, it just will not find repos.
	Exists bool `json:"exists"`
	// Repos counts immediate subdirectories holding a .git entry. It is the
	// cheap shape of hive's own scan — no git process per repository — so it
	// can run while a folder picker is open. It can read one high: hive skips
	// a repository it cannot read an origin remote from, and this does not.
	// That is the right trade for what it is for, which is confirming the
	// folder is the one the user meant, not promising a launch list.
	Repos int `json:"repos"`
}

// Setup is what the Hive config declares right now, and whether that is
// enough for the session launcher to do anything.
type Setup struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	// Usable reports that the file declares at least one agent profile and at
	// least one workspace. It is what first run branches on: a file that
	// exists but declares neither leaves the launcher exactly as empty as no
	// file at all.
	Usable bool `json:"usable"`
	// Unreadable carries a parse or read failure. The rest of Setup is then
	// zero, and the caller must not offer to rewrite the file — the user has
	// content here that this app could not understand.
	Unreadable string `json:"unreadable"`

	DefaultAgent string      `json:"defaultAgent"`
	Profiles     []Profile   `json:"profiles"`
	Workspaces   []Workspace `json:"workspaces"`
}

type document struct {
	Agents     yaml.Node `yaml:"agents"`
	Workspaces []string  `yaml:"workspaces"`
	// RepoDirs is hive's deprecated spelling of workspaces. It is read so an
	// older config reports as usable, but a write never produces it.
	RepoDirs []string `yaml:"repo_dirs"`
}

// Load reports what the config at path declares. A missing file is not an
// error: it is the ordinary first-run state, and reports Exists false.
func Load(path string) Setup {
	setup := Setup{Path: path}
	if path == "" {
		return setup
	}

	raw, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		return setup
	case err != nil:
		setup.Unreadable = err.Error()
		return setup
	}
	setup.Exists = true

	var doc document
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		setup.Unreadable = err.Error()
		return setup
	}

	setup.DefaultAgent, setup.Profiles = decodeAgents(doc.Agents)
	paths := doc.Workspaces
	if len(paths) == 0 {
		paths = doc.RepoDirs
	}
	for _, p := range paths {
		setup.Workspaces = append(setup.Workspaces, Inspect(p))
	}
	setup.Usable = len(setup.Profiles) > 0 && len(setup.Workspaces) > 0
	return setup
}

// Reading the raw mapping preserves declarations that Hive's unmarshaller
// would merge with defaults.
func decodeAgents(node yaml.Node) (defaultAgent string, profiles []Profile) {
	if node.Kind != yaml.MappingNode {
		return "", nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1]
		switch {
		case key == "default":
			defaultAgent = value.Value
		case config.IsReservedAgentKey(key):
		default:
			var p struct {
				Command string   `yaml:"command"`
				Flags   []string `yaml:"flags"`
			}
			if err := value.Decode(&p); err != nil {
				continue
			}
			command := p.Command
			if command == "" {
				command = key
			}
			profiles = append(profiles, Profile{Name: key, Command: command, Flags: p.Flags})
		}
	}
	return defaultAgent, profiles
}

// Inspect reports whether a workspace exists and counts its immediate Git
// repositories.
func Inspect(path string) Workspace {
	expanded := pathutil.ExpandHome(path)
	w := Workspace{Path: path}
	info, err := os.Stat(expanded)
	if err != nil || !info.IsDir() {
		return w
	}
	w.Exists = true
	entries, err := os.ReadDir(expanded)
	if err != nil {
		return w
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if _, err := os.Stat(filepath.Join(expanded, entry.Name(), ".git")); err == nil {
			w.Repos++
		}
	}
	return w
}

// LookPath resolves a command on the environment the app launches subprocesses
// with. It is satisfied by execenv.Resolver.LookPath (Consumer-defined
// interfaces, architecture.md).
type LookPath func(ctx context.Context, name string) (string, error)

// AgentOptions returns the catalog with each entry marked by whether its
// command resolves on the launch PATH. Order is the catalog's: installed
// agents are not floated to the top, because a picker whose rows move between
// launches is harder to use than one that does not.
func AgentOptions(ctx context.Context, lookPath LookPath) []AgentOption {
	known := agent.Known()
	options := make([]AgentOption, 0, len(known))
	for _, a := range known {
		installed := false
		if lookPath != nil {
			_, err := lookPath(ctx, a.Name)
			installed = err == nil
		}
		kind := AgentKind{Name: a.Name, Label: a.Label, SkipPermissionFlags: a.SkipPermissionFlags}
		options = append(options, AgentOption{AgentKind: kind, Installed: installed})
	}
	return options
}
