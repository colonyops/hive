// Package execenv resolves the environment the app runs subprocesses with, and
// answers what a variable would hold in the user's terminal.
//
// A desktop launch inherits the launcher's environment, not a shell's: macOS
// starts an .app bundle with PATH=/usr/bin:/bin:/usr/sbin:/sbin. Session hooks
// and shell actions are the user's own commands, written against the PATH their
// terminal has, so running them with the inherited one fails on anything a
// package manager, a version manager or a language toolchain installed
// (ADR subprocess-environment). The same gap hides hive's own environment
// overrides from the app, which is what Getenv answers
// (ADR hive-env-overrides-resolve-through-the-login-shell).
package execenv

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// The prefixes package managers install into, standing in for the PATH a
// desktop launch does not get. Order is precedence: a Homebrew binary is the
// one the user's shell would have picked over /usr/bin.
var systemDirs = []string{
	"/opt/homebrew/bin",                 // Homebrew, Apple silicon
	"/usr/local/bin",                    // Homebrew on Intel, manual installs
	"/opt/local/bin",                    // MacPorts
	"/home/linuxbrew/.linuxbrew/bin",    // Homebrew on Linux
	"/run/current-system/sw/bin",        // NixOS
	"/nix/var/nix/profiles/default/bin", // Nix, multi-user
	"/usr/bin",
	"/bin",
}

// SearchDirs returns those prefixes plus the per-user ones. It is where a
// binary lives when the environment does not say: tmuxbin searches the list for
// one executable (ADR tmux-discovery), and the resolver appends it to PATH.
func SearchDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return systemDirs
	}
	return slices.Concat(systemDirs, []string{
		filepath.Join(home, ".nix-profile", "bin"),
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, "bin"),
	})
}

// defaultTimeout bounds the login shell probe. A startup file that sources a
// version manager takes a noticeable fraction of this; one that hangs must not
// take session creation with it.
const defaultTimeout = 5 * time.Second

// probeCommand runs a program instead of echoing $PATH because the syntax to
// echo it is not portable: fish joins a list variable with spaces, so
// `printf %s $PATH` there produces something that is not a PATH at all.
// /usr/bin/env is absolute so a startup file that breaks PATH cannot hide it.
const probeCommand = "/usr/bin/env"

var errNoEnvironment = errors.New("shell reported no environment")

// Options configures a Resolver. The zero value probes $SHELL with the default
// timeout and logs nothing.
type Options struct {
	Logger zerolog.Logger
	// Shell is the login shell to ask. Empty reads $SHELL; still empty skips
	// the probe.
	Shell string
	// Timeout bounds one probe. Zero means defaultTimeout.
	Timeout time.Duration
	// Probe is the seam tests replace. Zero runs the real login shell.
	Probe func(ctx context.Context, shell string) (map[string]string, error)
}

// Resolver answers what PATH a subprocess runs with. It asks the user's login
// shell once per run and remembers the answer — a shell's PATH does not change
// under a running app, and re-running a login shell per hook command would
// charge every command that shell's startup cost.
type Resolver struct {
	logger  zerolog.Logger
	shell   string
	timeout time.Duration
	probe   func(ctx context.Context, shell string) (map[string]string, error)

	// mu serializes the probe as well as guarding path and env, so concurrent
	// hooks spawn one shell between them rather than one each.
	mu       sync.Mutex
	path     string
	env      map[string]string
	resolved bool

	// environ is the merge Environ last built, from environFrom. tmux is spawned
	// on every status poll, so the merge is rebuilt only when this process's
	// environment changes.
	environ     []string
	environFrom []string
}

func NewResolver(opts Options) *Resolver {
	r := &Resolver{logger: opts.Logger, shell: opts.Shell, timeout: opts.Timeout, probe: opts.Probe}
	if r.shell == "" {
		r.shell = os.Getenv("SHELL")
	}
	if r.timeout <= 0 {
		r.timeout = defaultTimeout
	}
	if r.probe == nil {
		r.probe = shellEnvironment
	}
	return r
}

// Path returns the PATH value subprocesses run with: what the login shell
// reports, then what this process inherited, then the package-manager prefixes,
// first occurrence winning. A probe that fails is not fatal — the inherited
// PATH still runs everything in /usr/bin, which is what the app had before.
func (r *Resolver) Path(ctx context.Context) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolveLocked(ctx)
	return r.path
}

// Getenv answers a variable the way the user's terminal would: this process's
// value when it has one, otherwise the login shell's. A variable exported from
// a startup file is invisible to a launched .app, so a config override the CLI
// reads from the environment is only reachable through the probe
// (ADR hive-env-overrides-resolve-through-the-login-shell).
func (r *Resolver) Getenv(ctx context.Context, name string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolveLocked(ctx)
	return r.env[name]
}

func (r *Resolver) resolveLocked(ctx context.Context) {
	if r.resolved {
		return
	}

	inherited := os.Getenv("PATH")
	env, err := r.resolveShellEnvironment(ctx)
	switch {
	case err != nil:
		r.logger.Warn().Ctx(ctx).Err(err).Str("shell", r.shell).
			Msg("login shell environment unavailable; hook and shell-action commands run with the inherited PATH")
	case len(env) == 0:
		// No shell to ask. A shell that answers nothing errors instead.
	case env["PATH"] == "":
		r.logger.Warn().Ctx(ctx).Str("shell", r.shell).
			Msg("login shell reported no PATH; hook and shell-action commands run with the inherited PATH")
	default:
		r.logger.Info().Ctx(ctx).Str("shell", r.shell).Msg("resolved subprocess PATH from the login shell")
	}

	r.env = env
	r.path = join(env["PATH"], inherited, strings.Join(SearchDirs(), string(os.PathListSeparator)))
	r.resolved = true
	r.logger.Debug().Ctx(ctx).Str("path", r.path).Msg("subprocess PATH")
}

// LookPath resolves a command name against the same PATH its child will run
// with. It exists because os/exec does not: exec.Command searches the PATH of
// the process that calls it and ignores Cmd.Env, which is the one this package
// is replacing. A name that is already a path is returned untouched.
func (r *Resolver) LookPath(ctx context.Context, name string) (string, error) {
	if strings.ContainsRune(name, filepath.Separator) {
		return name, nil
	}
	for _, dir := range filepath.SplitList(r.Path(ctx)) {
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%q: %w", name, exec.ErrNotFound)
}

// shellSessionVars describe the probe shell's own process rather than anything
// a child should inherit: its nesting depth, its last command, its directory,
// the terminal it did not have, the tmux client it was attached to, and the
// shell it is — Options.Shell picks what gets probed, which need not be the
// shell a child should be told the user runs. Copying TMUX in particular tells
// a spawned process it is inside a tmux client that it is not. The list is
// closed because these are wrong by construction — every other variable a
// startup file exports is adopted
// (ADR a-subprocess-inherits-the-whole-shell-environment-not-just-its-path).
var shellSessionVars = map[string]bool{
	"SHLVL": true, "_": true,
	"PWD": true, "OLDPWD": true,
	"TERM": true, "TMUX": true, "TMUX_PANE": true,
	"SHELL": true,
}

// Environ returns the environment subprocesses run with: this process's, with
// PATH replaced by Path and the login shell's remaining variables filling names
// this process does not define. A launch that names a variable is more specific
// than a startup file, so the process wins — which keeps a stale rc file from
// shadowing a HIVE_DESKTOP_* override the app was started with.
//
// The result is shared between callers: append to it, never write into it.
func (r *Resolver) Environ(ctx context.Context) []string {
	current := os.Environ()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolveLocked(ctx)
	if r.environ == nil || !slices.Equal(current, r.environFrom) {
		r.environ = mergeEnviron(current, r.env, r.path)
		r.environFrom = current
	}
	return slices.Clip(r.environ)
}

func mergeEnviron(current []string, shell map[string]string, path string) []string {
	env := make([]string, 0, len(current)+len(shell)+1)
	// Presence wins, not a non-empty value: setting a variable to nothing is how
	// overrides.env opts out of a default, and a startup file must not refill it.
	defined := make(map[string]bool, len(current))
	for _, kv := range current {
		name, _, _ := strings.Cut(kv, "=")
		defined[name] = true
		if name != "PATH" {
			env = append(env, kv)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(shell)) {
		if name == "PATH" || defined[name] || shellSessionVars[name] {
			continue
		}
		env = append(env, name+"="+shell[name])
	}
	return append(env, "PATH="+path)
}

func (r *Resolver) resolveShellEnvironment(ctx context.Context) (map[string]string, error) {
	if r.shell == "" {
		return nil, nil
	}
	// The probe outlives the caller's cancellation deliberately: it runs once
	// per app run, and a session creation cancelled mid-probe would otherwise
	// record a failure that had nothing to do with the shell.
	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.timeout)
	defer cancel()
	return r.probe(probeCtx, r.shell)
}

// probeKillGrace bounds how long the probe waits on its output pipe after the
// shell is done. The context only bounds the shell's lifetime: a startup file
// that background-spawns anything inheriting stdout (`something &` in .zshrc)
// keeps the pipe open after the shell exits, and Output would block on it —
// with the resolver's lock held, stalling every spawn in the app. WaitDelay
// force-closes the pipe instead; the same mechanism as dispatch's
// shellKillGrace.
const probeKillGrace = 2 * time.Second

// shellEnvironment asks the user's login shell for its environment. Interactive
// (-i) as well as login (-l), because the PATH a terminal shows is as often set
// in an interactive startup file (.zshrc) as in a login one.
func shellEnvironment(ctx context.Context, shell string) (map[string]string, error) {
	cmd := exec.CommandContext(ctx, shell, "-ilc", probeCommand)
	// An interactive shell enables job control and can make itself the foreground
	// process group of an inherited terminal. It would leave that terminal aimed
	// at a dead group when this one-shot probe exits, so later Ctrl+C presses
	// never reach the app or its dev supervisor.
	isolateShellProbe(cmd)
	cmd.WaitDelay = probeKillGrace
	out, err := cmd.Output()
	// ErrWaitDelay means the shell exited cleanly but something it left
	// running held the pipe past the grace. Its answer is already in out —
	// a shell that answered is not failed for what it left behind.
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		return nil, err
	}
	env := make(map[string]string)
	// A startup file is free to print before env runs, and a value is free to
	// span lines, so a line is an assignment only when nothing before its first
	// `=` is whitespace; anything else continues the value above it. current is
	// the name those continuations belong to, and is cleared by an assignment
	// that is dropped so a rejected value's own lines go with it.
	current := ""
	for line := range strings.SplitSeq(strings.Trim(string(out), "\n"), "\n") {
		name, value, isAssignment := strings.Cut(line, "=")
		switch {
		case isAssignment && !strings.ContainsAny(name, " \t"):
			current = ""
			// bash exports functions as BASH_FUNC_x%%=() {…}, whose value spans
			// lines env gives no way to delimit. Adopting the first line of one
			// hands every child a definition its shell then fails to parse, so
			// a name that is not a name is not taken at all.
			if isEnvName(name) {
				env[name] = value
				current = name
			}
		case current != "":
			env[current] += "\n" + line
		}
	}
	if len(env) == 0 {
		return nil, errNoEnvironment
	}
	return env, nil
}

// isEnvName reports whether name is a portable environment variable name.
func isEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		alpha := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		digit := i > 0 && r >= '0' && r <= '9'
		if !alpha && !digit {
			return false
		}
	}
	return true
}

// join concatenates PATH values, dropping empty and repeated entries so the
// result is what a shell would have searched, in that order, once each.
func join(lists ...string) string {
	separator := string(os.PathListSeparator)
	seen := make(map[string]bool)
	dirs := make([]string, 0, len(lists)*8)
	for _, list := range lists {
		for dir := range strings.SplitSeq(list, separator) {
			if dir == "" || seen[dir] {
				continue
			}
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	return strings.Join(dirs, separator)
}
