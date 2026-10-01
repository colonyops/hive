// Package devproxy is the contract shared by cmd/desktop/devserver and cmd/desktop/devtools:
// where the development GitHub proxy listens, and how to tell whether one is
// already running there.
//
// It exists because the proxy is a singleton by design (ADR devserver-github-proxy). devserver
// needs to know if it is a duplicate launch, and devtools needs to point a
// worktree's launch.env at the same address and fail loudly when nothing is
// listening. Duplicating either fact would let the two drift into a setup
// where the app is redirected at a port no proxy owns.
package devproxy

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultListen is the proxy's bind address when the config omits one. It is a
// fixed loopback port rather than an allocated one precisely because every
// instance has to find it without discovery.
const DefaultListen = "127.0.0.1:7777"

// RepoConfigPath is the checked-in development config, relative to the
// repository root. Development configuration lives in the repo, not in a
// user's home directory: it is versioned with the code whose behaviour it
// simulates, and a fresh clone gets working scenarios with no setup step.
const RepoConfigPath = "cmd/desktop/devserver/devserver.yaml"

// HealthPath answers the "are you already running?" probe. It is deliberately
// cheaper than /_ctl/state, which builds a full overlay and scenario snapshot.
const HealthPath = "/_ctl/health"

// Health is the probe payload. The Devserver field is the marker that
// distinguishes our proxy from any other process holding the port — binding
// blindly over a stranger's listener would be a confusing way to fail.
type Health struct {
	Devserver bool `json:"devserver"`
}

// probeTimeout bounds the health probe. The target is always loopback, so a
// slow answer means something is wrong rather than far away.
const probeTimeout = 2 * time.Second

// Status is what a probe found at an address.
type Status int

const (
	// StatusAbsent means nothing is listening: this process may bind.
	StatusAbsent Status = iota
	// StatusRunning means our devserver already owns the address.
	StatusRunning
	// StatusForeign means something answered but is not devserver.
	StatusForeign
)

// BaseURL is the API base a desktop instance sets to route through a proxy
// listening at listen.
func BaseURL(listen string) string { return "http://" + listen }

// Probe reports what is answering at baseURL. A dial failure is StatusAbsent
// rather than an error: "nothing there" is the normal case for both callers,
// and only a caller can say whether it is a problem.
func Probe(ctx context.Context, baseURL string) Status {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(baseURL, "/")+HealthPath, nil)
	if err != nil {
		return StatusAbsent
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return StatusAbsent
	}
	defer resp.Body.Close() //nolint:errcheck // probe response body is discarded

	if resp.StatusCode != http.StatusOK {
		return StatusForeign
	}
	var health Health
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil || !health.Devserver {
		return StatusForeign
	}
	return StatusRunning
}

// ListenFromConfig reads the bind address out of the checked-in config at
// root, falling back to DefaultListen when the file is absent or declares no
// listen. The config is the single source of truth for the port, so a change
// there reaches launch.env without anyone editing two places.
func ListenFromConfig(root string) string {
	data, err := os.ReadFile(filepath.Join(root, RepoConfigPath))
	if err != nil {
		return DefaultListen
	}
	var parsed struct {
		Listen string `yaml:"listen"`
	}
	if err := yaml.Unmarshal(data, &parsed); err != nil || parsed.Listen == "" {
		return DefaultListen
	}
	return parsed.Listen
}

// EnvAPIBase names the desktop setting this package's address feeds. It is
// duplicated from cmd/desktop/internal/app/settings rather than imported because
// cmd/desktop/devserver must not depend on an app package for a string (ADR devserver-github-proxy: the
// app does not import devserver and devserver does not import the app), and
// both binaries need the name — devtools to read it, devserver to name it in
// the hint it prints at startup. cmd/desktop/devtools imports both packages and asserts
// the two spellings agree.
const EnvAPIBase = "HIVE_DESKTOP_DEVELOPMENT_GITHUB_API_BASE"
