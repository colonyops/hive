package app

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/hivecli"
	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
	"github.com/colonyops/hive/internal/ops/catalog"
	"github.com/colonyops/hive/internal/platform/execenv"
)

const driveServePath = "/mcp/drive"

// HiveCLI reports the hive binary an agent config can launch as an MCP server.
type HiveCLI interface {
	// Usable returns the absolute path of a hive binary built from the same
	// version as this desktop, and false when there is none.
	Usable(ctx context.Context) (string, bool)
}

// hiveCLIResolver resolves hive the way the daemon does (daemon.hive_path,
// else the login-shell PATH). `hive mcp serve` proxies to this desktop, so a
// binary of another version could speak a different operation catalogue.
type hiveCLIResolver struct {
	configuredPath func(context.Context) string
	execEnv        *execenv.Resolver
	version        string
	logger         zerolog.Logger
	// probe returns the output of `hive --version`.
	probe func(ctx context.Context, path string, env []string) (string, error)

	mu    sync.Mutex
	cache map[hiveProbeKey]bool
}

// hiveProbeKey includes the mtime so an upgraded binary is probed again.
type hiveProbeKey struct {
	path  string
	mtime time.Time
}

func newHiveCLIResolver(configuredPath func(context.Context) string, execEnv *execenv.Resolver, version string, logger zerolog.Logger) *hiveCLIResolver {
	return &hiveCLIResolver{
		configuredPath: configuredPath,
		execEnv:        execEnv,
		version:        version,
		logger:         logger,
		probe: func(ctx context.Context, path string, env []string) (string, error) {
			return hivecli.RunVersion(ctx, path, env, 5*time.Second)
		},
		cache: map[hiveProbeKey]bool{},
	}
}

func (r *hiveCLIResolver) Usable(ctx context.Context) (string, bool) {
	path := r.configuredPath(ctx)
	if path == "" {
		found, err := r.execEnv.LookPath(ctx, "hive")
		if err != nil {
			r.logger.Debug().Err(err).Msg("hive MCP stdio unavailable: hive not found")
			return "", false
		}
		path = found
	}
	info, err := os.Stat(path)
	if err != nil {
		r.logger.Debug().Err(err).Str("path", path).Msg("hive MCP stdio unavailable: cannot stat hive")
		return "", false
	}
	key := hiveProbeKey{path: path, mtime: info.ModTime()}

	r.mu.Lock()
	defer r.mu.Unlock()
	if ok, cached := r.cache[key]; cached {
		return path, ok
	}
	out, err := r.probe(ctx, path, r.execEnv.Environ(ctx))
	if err != nil {
		r.logger.Debug().Err(err).Str("path", path).Msg("hive MCP stdio unavailable: version probe failed")
		return "", false
	}
	got, _ := hivecli.ParseVersion(out)
	ok := got != "" && got == r.version
	if !ok {
		r.logger.Debug().Str("path", path).Str("hive", got).Str("desktop", r.version).Msg("hive MCP stdio unavailable: version differs")
	}
	r.cache[key] = ok
	if !ok {
		return "", false
	}
	return path, true
}

// serveToolsets lists the toolsets `hive mcp serve` serves for servePath,
// read from the operation catalogue so a new toolset needs no edit here.
func serveToolsets(servePath string) []string {
	var names []string
	for _, op := range catalog.All() {
		toolset := op.Info().Toolset
		if toolset.MCPPath == servePath {
			names = append(names, toolset.Name)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// stdioServe is a usable hive plus the data directory it serves from.
type stdioServe struct {
	hive    string
	dataDir string
}

func (s stdioServe) args(toolsets []string) []string {
	return []string{"--data-dir", s.dataDir, "mcp", "serve", "--toolset", strings.Join(toolsets, ",")}
}

func (s stdioServe) server(toolsets []string) mcpcatalog.Server {
	return mcpcatalog.Server{Transport: mcpcatalog.TransportStdio, Command: s.hive, Args: s.args(toolsets)}
}

// driveMCPCommand builds the `mcp add` argv for an agent CLI. A nil stdio
// registers the HTTP url.
func driveMCPCommand(agent, url string, stdio *stdioServe) (string, []string, error) {
	var serve []string
	if stdio != nil {
		serve = append([]string{"--", stdio.hive}, stdio.args(serveToolsets(driveServePath))...)
	}
	switch agent {
	case "claude":
		if stdio != nil {
			return "claude", append([]string{"mcp", "add", "--scope", "user", "hive-drive"}, serve...), nil
		}
		return "claude", []string{"mcp", "add", "--scope", "user", "--transport", "http", "hive-drive", url}, nil
	case "codex":
		if stdio != nil {
			return "codex", append([]string{"mcp", "add", "hive-drive"}, serve...), nil
		}
		return "codex", []string{"mcp", "add", "hive-drive", "--url", url}, nil
	}
	return "", nil, Errorf(KindInvalid, "MCP setup is not supported for agent %q", agent)
}
