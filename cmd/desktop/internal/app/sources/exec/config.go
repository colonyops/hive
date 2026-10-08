package exec

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/icons"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sourcemark"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/pkg/duration"
)

// maxTimeout bounds a run. One tick drains every pull source in sequence, so a
// command's timeout is spent out of every other source's freshness — and the
// default poll interval is five minutes. Two minutes is comfortably above any
// command worth polling and keeps the worst case under a tick.
const maxTimeout = 2 * time.Minute

// maxEnvVars bounds the per-node environment. The resolved environment is
// already inherited; this map is for the handful of variables a command needs
// on top of it.
const maxEnvVars = 32

// Config is a command source node's configuration.
//
// The command is a fixed string, never a template: nothing ingested, fetched,
// or otherwise off-machine can influence what runs. See ADR a-command-is-a-source — that
// invariant is what keeps this node as trusted as the file it is written in
// and no more.
type Config struct {
	// Command is the shell command line, run through `sh -c`.
	Command string `json:"command" yaml:"command" jsonschema:"title=Command,description=The shell command line to run each poll. It runs through 'sh -c' in the resolved PATH so pipes and redirection work; it must print a JSON array of item objects on stdout and exit 0."`
	// Timeout bounds one run. Required: a command with no deadline can hang
	// the poll loop, and a source that silently stopped producing is the
	// failure this node exists to remove.
	Timeout duration.Duration `json:"timeout" yaml:"timeout" jsonschema:"title=Timeout,description=How long one run may take before it is killed and the tick fails. Required; at most 2m because a tick drains sources in sequence."`
	// Cwd is the directory the command runs in. Empty is the desktop
	// process's own, which for a launched .app bundle is not a useful place.
	Cwd string `json:"cwd,omitempty" yaml:"cwd,omitempty" jsonschema:"title=Working directory,description=Absolute path (or one starting with ~/) the command runs in. Empty runs in the app's own directory."`
	// Env adds to the resolved environment for this command only.
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty" jsonschema:"title=Environment,description=Extra environment variables for this command, added to the resolved environment. Values are literal; nothing is expanded or interpolated."`
	// Interval is the floor between runs, for a command that is expensive or
	// only worth running hourly. Empty runs it on every poll tick.
	Interval duration.Duration `json:"interval,omitempty" yaml:"interval,omitempty" jsonschema:"title=Minimum interval,description=Shortest time between runs. The command still only runs on a poll tick so the real cadence rounds up to the next one; empty runs it on every tick."`
	// Icon is the glyph feed rows render for this node's items, from the
	// shared feed icon set. Purely cosmetic; empty means the default.
	Icon string `json:"icon,omitempty" yaml:"icon,omitempty" jsonschema:"title=Icon,description=The glyph feed rows render for this node's items. Empty uses the default command glyph."`
	// Image, when set, is the content hash of an uploaded image shown as this
	// node's feed mark instead of Icon (see cmd/desktop/internal/app/sourcemark). Empty or a
	// missing file falls back to Icon.
	Image string `json:"image,omitempty" yaml:"image,omitempty" jsonschema:"title=Image,description=Content hash of an uploaded image shown as this source's feed mark instead of the icon. Set through the node editor's image picker; empty falls back to the icon."`
}

// MarkImage and SetMarkImage read and record the feed-mark image hash, satisfying
// the flow store's image-mark interface.
func (c *Config) MarkImage() string        { return c.Image }
func (c *Config) SetMarkImage(hash string) { c.Image = hash }

// Validate checks the command is present, the timeout is set and bounded, and
// the working directory is one a process can actually chdir to.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Command) == "" {
		return fmt.Errorf("exec source: command is required")
	}
	switch timeout := c.Timeout.Duration(); {
	case timeout <= 0:
		return fmt.Errorf("exec source: timeout is required, like \"30s\"")
	case timeout > maxTimeout:
		return fmt.Errorf("exec source: timeout %s exceeds the maximum %s", c.Timeout, duration.Duration(maxTimeout))
	}
	if err := connector.ValidateInterval("exec source", c.Interval); err != nil {
		return err
	}
	if cwd := c.Cwd; cwd != "" && !filepath.IsAbs(cwd) && !strings.HasPrefix(cwd, "~/") && cwd != "~" {
		return fmt.Errorf("exec source: cwd %q must be an absolute path (a relative one resolves against the app's directory, not yours)", cwd)
	}
	if len(c.Env) > maxEnvVars {
		return fmt.Errorf("exec source: env declares %d variables, at most %d", len(c.Env), maxEnvVars)
	}
	for name := range c.Env {
		if name == "" || strings.ContainsAny(name, "= \t\n\x00") {
			return fmt.Errorf("exec source: env name %q is not a usable variable name", name)
		}
	}
	if !icons.ValidFeed(c.Icon) {
		return fmt.Errorf("exec source: icon %q is not a supported feed icon", c.Icon)
	}
	if c.Image != "" && !sourcemark.ValidHash(c.Image) {
		return fmt.Errorf("exec source: image %q is not a valid mark reference", c.Image)
	}
	return nil
}
