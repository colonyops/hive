package execenv

import (
	"context"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pathEntries(t *testing.T, value string) []string {
	t.Helper()
	return strings.Split(value, string(os.PathListSeparator))
}

func TestPathPrefersTheLoginShell(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")

	r := NewResolver(Options{Shell: "/bin/zsh", Probe: func(context.Context, string) (map[string]string, error) {
		return map[string]string{"PATH": "/opt/tools/bin:/usr/bin"}, nil
	}})

	entries := pathEntries(t, r.Path(t.Context()))
	require.Equal(t, "/opt/tools/bin", entries[0], "the shell's PATH comes first")
	assert.Contains(t, entries, "/usr/bin")
	assert.Contains(t, entries, "/opt/homebrew/bin", "the package-manager prefixes still backstop it")
	assert.Equal(t, entries, slicesUnique(entries), "a directory both sides name is searched once")
}

// A probe that fails is the common case on a machine with an exotic shell, so
// it must degrade to what the app could already reach rather than to nothing.
func TestPathFallsBackWhenTheProbeFails(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")

	r := NewResolver(Options{Shell: "/bin/zsh", Probe: func(context.Context, string) (map[string]string, error) {
		return nil, errors.New("exit status 1")
	}})

	entries := pathEntries(t, r.Path(t.Context()))
	assert.Equal(t, []string{"/usr/bin", "/bin"}, entries[:2])
	assert.Contains(t, entries, "/opt/homebrew/bin")
}

func TestPathWithoutAShellIsTheInheritedPathPlusPrefixes(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("SHELL", "")

	probed := false
	r := NewResolver(Options{Probe: func(context.Context, string) (map[string]string, error) {
		probed = true
		return map[string]string{"PATH": "/never"}, nil
	}})

	entries := pathEntries(t, r.Path(t.Context()))
	assert.False(t, probed, "there is no shell to ask")
	assert.Equal(t, "/usr/bin", entries[0])
	assert.Contains(t, entries, "/opt/homebrew/bin")
}

// The shell is asked once per run: its PATH does not change under a running
// app, and hooks arrive one command at a time.
func TestPathProbesOnceAcrossConcurrentCallers(t *testing.T) {
	var probes atomic.Int64
	r := NewResolver(Options{Shell: "/bin/zsh", Probe: func(context.Context, string) (map[string]string, error) {
		probes.Add(1)
		time.Sleep(10 * time.Millisecond)
		return map[string]string{"PATH": "/opt/tools/bin"}, nil
	}})

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			assert.Contains(t, r.Path(t.Context()), "/opt/tools/bin")
		})
	}
	wg.Wait()

	assert.Equal(t, int64(1), probes.Load())
}

// A failed probe is remembered too. Retrying would charge every later hook
// command the startup cost of a shell that already declined to answer.
func TestPathRemembersAFailedProbe(t *testing.T) {
	var probes atomic.Int64
	r := NewResolver(Options{Shell: "/bin/zsh", Probe: func(context.Context, string) (map[string]string, error) {
		probes.Add(1)
		return nil, errors.New("no")
	}})

	require.NotEmpty(t, r.Path(t.Context()))
	require.NotEmpty(t, r.Path(t.Context()))
	assert.Equal(t, int64(1), probes.Load())
}

// Cancelling session creation must not be recorded as the shell's answer, so
// the probe runs on a context of its own.
func TestProbeOutlivesACancelledCaller(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	r := NewResolver(Options{Shell: "/bin/zsh", Probe: func(ctx context.Context, _ string) (map[string]string, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return map[string]string{"PATH": "/opt/tools/bin"}, nil
	}})

	assert.Contains(t, r.Path(ctx), "/opt/tools/bin")
}

func TestProbeIsBoundedByTheTimeout(t *testing.T) {
	r := NewResolver(Options{Shell: "/bin/zsh", Timeout: 20 * time.Millisecond, Probe: func(ctx context.Context, _ string) (map[string]string, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}})

	done := make(chan string, 1)
	go func() { done <- r.Path(t.Context()) }()
	select {
	case path := <-done:
		assert.NotEmpty(t, path)
	case <-time.After(5 * time.Second):
		t.Fatal("a hanging shell must not hang session creation")
	}
}

func environMap(t *testing.T, env []string) map[string]string {
	t.Helper()
	out := make(map[string]string, len(env))
	for _, kv := range env {
		name, value, ok := strings.Cut(kv, "=")
		require.True(t, ok, "every entry is an assignment")
		_, duplicate := out[name]
		require.False(t, duplicate, "%s reaches the child once", name)
		out[name] = value
	}
	return out
}

func TestEnvironCarriesThePathAndThisProcess(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HIVE_EXECENV_MARKER", "kept")

	r := NewResolver(Options{Shell: "/bin/zsh", Probe: func(context.Context, string) (map[string]string, error) {
		return map[string]string{"PATH": "/opt/tools/bin"}, nil
	}})

	env := environMap(t, r.Environ(t.Context()))
	assert.Equal(t, r.Path(t.Context()), env["PATH"])
	assert.Equal(t, "kept", env["HIVE_EXECENV_MARKER"], "the rest of the environment is passed through")
}

// EDITOR set in .zshrc is the case that motivated adopting more than PATH: an
// agent CLI's "open in editor" had nothing to resolve and fell back to whatever
// was on PATH (#279).
func TestEnvironAdoptsShellVariablesThisProcessLacks(t *testing.T) {
	t.Setenv("HIVE_EXECENV_EDITOR", "")
	require.NoError(t, os.Unsetenv("HIVE_EXECENV_EDITOR"), "t.Setenv registers the restore; this makes it absent")

	r := NewResolver(Options{Shell: "/bin/zsh", Probe: func(context.Context, string) (map[string]string, error) {
		return map[string]string{"PATH": "/opt/tools/bin", "HIVE_EXECENV_EDITOR": "nvim"}, nil
	}})

	env := environMap(t, r.Environ(t.Context()))
	assert.Equal(t, "nvim", env["HIVE_EXECENV_EDITOR"])
}

// How the app was launched is more specific than what a startup file exports,
// so a stale rc file cannot shadow a HIVE_DESKTOP_* override the app was
// started with. An empty value counts as defined: setting one to nothing is how
// overrides.env opts out of a default.
func TestEnvironPrefersThisProcessOverTheShell(t *testing.T) {
	t.Setenv("HIVE_EXECENV_OVERRIDE", "from-launch")
	t.Setenv("HIVE_EXECENV_OPT_OUT", "")

	r := NewResolver(Options{Shell: "/bin/zsh", Probe: func(context.Context, string) (map[string]string, error) {
		return map[string]string{
			"HIVE_EXECENV_OVERRIDE": "from-rc-file",
			"HIVE_EXECENV_OPT_OUT":  "from-rc-file",
		}, nil
	}})

	env := environMap(t, r.Environ(t.Context()))
	assert.Equal(t, "from-launch", env["HIVE_EXECENV_OVERRIDE"])
	optOut, present := env["HIVE_EXECENV_OPT_OUT"]
	require.True(t, present, "the empty assignment itself reaches the child; unset would mean something else")
	assert.Empty(t, optOut, "an explicit opt-out is not refilled by a startup file")
}

// The merge is reused across calls, which must not freeze it: the updater
// redirects TMPDIR while it stages, and a child spawned then or after has to see
// the environment as it is.
func TestEnvironFollowsChangesToThisProcess(t *testing.T) {
	t.Setenv("HIVE_EXECENV_LATER", "before")
	r := NewResolver(Options{Shell: "/bin/zsh", Probe: func(context.Context, string) (map[string]string, error) {
		return map[string]string{"PATH": "/opt/tools/bin"}, nil
	}})
	first := r.Environ(t.Context())
	assert.Equal(t, "before", environMap(t, first)["HIVE_EXECENV_LATER"])

	t.Setenv("HIVE_EXECENV_LATER", "after")

	assert.Equal(t, "after", environMap(t, r.Environ(t.Context()))["HIVE_EXECENV_LATER"])
	assert.Equal(t, "before", environMap(t, first)["HIVE_EXECENV_LATER"], "an earlier caller's slice is not rewritten")
}

// sources/exec appends a node's own variables to what Environ returns, so one
// caller's append must never land in the next caller's environment.
func TestEnvironAppendsDoNotLeakBetweenCallers(t *testing.T) {
	r := NewResolver(Options{Shell: "/bin/zsh", Probe: func(context.Context, string) (map[string]string, error) {
		return map[string]string{"PATH": "/opt/tools/bin"}, nil
	}})

	_ = append(r.Environ(t.Context()), "HIVE_EXECENV_NODE=one")

	_, leaked := environMap(t, r.Environ(t.Context()))["HIVE_EXECENV_NODE"]
	assert.False(t, leaked)
}

// These describe the probe shell's own session, not the child's. TMUX is the
// one that does damage: it tells a spawned process it is inside a tmux client
// that it is not.
func TestEnvironDropsTheProbeShellsSessionVariables(t *testing.T) {
	// The literal list, not the production map: a test that ranges over
	// shellSessionVars loses an entry's coverage the moment the entry is
	// dropped, which is the regression it exists to catch.
	names := []string{"SHLVL", "_", "PWD", "OLDPWD", "TERM", "TMUX", "TMUX_PANE", "SHELL"}
	require.ElementsMatch(t, names, slices.Collect(maps.Keys(shellSessionVars)))

	// A name this process defines is blocked by precedence before the denylist
	// is consulted, so each one is made absent to leave the denylist standing
	// alone — most of these are defined in any environment that runs go test.
	probed := map[string]string{"PATH": "/opt/tools/bin", "HIVE_EXECENV_MARKER": "adopted"}
	for _, name := range names {
		t.Setenv(name, "restored-after")
		require.NoError(t, os.Unsetenv(name))
		probed[name] = "probe-shell-" + name
	}
	r := NewResolver(Options{Shell: "/bin/zsh", Probe: func(context.Context, string) (map[string]string, error) {
		return probed, nil
	}})

	env := environMap(t, r.Environ(t.Context()))
	require.Equal(t, "adopted", env["HIVE_EXECENV_MARKER"], "a variable outside the list is still adopted")
	for _, name := range names {
		_, present := env[name]
		assert.False(t, present, "%s describes the probe shell, not the child", name)
	}
}

// A variable the user exports from a startup file is invisible to a launched
// .app, so the probe is the only place the app can read one from.
func TestGetenvFallsBackToTheLoginShell(t *testing.T) {
	t.Setenv("HIVE_DEFAULT_AGENT", "")

	r := NewResolver(Options{Shell: "/bin/zsh", Probe: func(context.Context, string) (map[string]string, error) {
		return map[string]string{"PATH": "/opt/tools/bin", "HIVE_DEFAULT_AGENT": "codex"}, nil
	}})

	assert.Equal(t, "codex", r.Getenv(t.Context(), "HIVE_DEFAULT_AGENT"))
	assert.Empty(t, r.Getenv(t.Context(), "HIVE_NOT_SET"), "a variable neither side has is unset")
}

// How the app was launched is more specific than what a startup file exports,
// so `HIVE_DEFAULT_AGENT=codex open -a …` wins over the shell's answer.
func TestGetenvPrefersThisProcess(t *testing.T) {
	t.Setenv("HIVE_DEFAULT_AGENT", "pi")

	probed := false
	r := NewResolver(Options{Shell: "/bin/zsh", Probe: func(context.Context, string) (map[string]string, error) {
		probed = true
		return map[string]string{"HIVE_DEFAULT_AGENT": "codex"}, nil
	}})

	assert.Equal(t, "pi", r.Getenv(t.Context(), "HIVE_DEFAULT_AGENT"))
	assert.False(t, probed, "the answer was already known; no shell is started for it")
}

// The real probe, against a stub standing in for the user's shell: it must read
// the child's own environment rather than trust the shell to echo a variable.
func TestShellEnvironmentReadsTheShellsEnvironment(t *testing.T) {
	shell := filepath.Join(t.TempDir(), "shell")
	require.NoError(t, os.WriteFile(shell, []byte(
		"#!/bin/sh\n"+
			"echo 'startup file noise'\n"+
			"export PATH=/opt/tools/bin:/usr/bin\n"+
			"export HIVE_DEFAULT_AGENT=codex\n"+
			"exec \"$2\"\n"), 0o755))

	env, err := shellEnvironment(t.Context(), shell)
	require.NoError(t, err)
	assert.Equal(t, "/opt/tools/bin:/usr/bin", env["PATH"])
	assert.Equal(t, "codex", env["HIVE_DEFAULT_AGENT"], "a variable other than PATH is kept too")
}

// A startup file may leave a background process holding the shell's stdout
// (`something &` in .zshrc). The shell's own exit must end the probe anyway —
// otherwise the resolver's lock is held until that stranger exits.
func TestShellEnvironmentReturnsWhenAChildHoldsThePipeOpen(t *testing.T) {
	shell := filepath.Join(t.TempDir(), "shell")
	require.NoError(t, os.WriteFile(shell, []byte(
		"#!/bin/sh\n"+
			"export PATH=/opt/tools/bin:/usr/bin\n"+
			"/bin/sleep 10 &\n"+
			"exec \"$2\"\n"), 0o755))

	start := time.Now()
	env, err := shellEnvironment(t.Context(), shell)
	require.NoError(t, err)
	assert.Equal(t, "/opt/tools/bin:/usr/bin", env["PATH"])
	assert.Less(t, time.Since(start), 8*time.Second,
		"the probe must end with the shell, not with whatever it left running")
}

// bash exports a function as BASH_FUNC_x%%=() {…}, a value spanning lines that
// env gives no way to delimit. Its first line is not a usable definition, and
// its remaining lines must not land on the variable printed before it.
func TestShellEnvironmentDropsExportedFunctions(t *testing.T) {
	shell := filepath.Join(t.TempDir(), "shell")
	require.NoError(t, os.WriteFile(shell, []byte(
		"#!/bin/sh\n"+
			"printf 'PATH=/opt/tools/bin\\n'\n"+
			"printf 'EDITOR=nvim\\n'\n"+
			"printf 'BASH_FUNC_greet%%%%=() {  echo hi\\n}\\n'\n"+
			"printf 'PAGER=less\\n'\n"), 0o755))

	env, err := shellEnvironment(t.Context(), shell)
	require.NoError(t, err)
	assert.Equal(t, "nvim", env["EDITOR"], "the variable before a function keeps its own value")
	assert.Equal(t, "less", env["PAGER"], "parsing resumes after one")
	for name := range env {
		assert.NotContains(t, name, "BASH_FUNC", "a name that is not a name is not taken")
	}
}

// A value is free to span lines, and the lines after the first belong to it —
// adopting only the first would hand every child a truncated value.
func TestShellEnvironmentKeepsMultiLineValuesWhole(t *testing.T) {
	shell := filepath.Join(t.TempDir(), "shell")
	require.NoError(t, os.WriteFile(shell, []byte(
		"#!/bin/sh\n"+
			"echo 'sourcing rc: mode=verbose'\n"+
			"printf 'PATH=/opt/tools/bin\\n'\n"+
			"printf 'GREETING=hello\\nnote: level=verbose\\nworld\\n'\n"), 0o755))

	env, err := shellEnvironment(t.Context(), shell)
	require.NoError(t, err)
	assert.Equal(t, "hello\nnote: level=verbose\nworld", env["GREETING"],
		"a continuation line holding an = still belongs to the open value")
	assert.NotContains(t, env, "sourcing rc: mode", "printed noise is not an assignment")
	assert.Len(t, env, 2)
}

func TestIsEnvName(t *testing.T) {
	assert.True(t, isEnvName("EDITOR"))
	assert.True(t, isEnvName("_MISE_2"))
	assert.False(t, isEnvName(""))
	assert.False(t, isEnvName("2FA"), "a leading digit is not a name")
	assert.False(t, isEnvName("BASH_FUNC_greet%%"))
	assert.False(t, isEnvName("café"), "multi-byte runes are not portable names")
}

func TestShellEnvironmentFailsWhenTheShellReportsNothing(t *testing.T) {
	shell := filepath.Join(t.TempDir(), "shell")
	require.NoError(t, os.WriteFile(shell, []byte("#!/bin/sh\necho hello\n"), 0o755))

	_, err := shellEnvironment(t.Context(), shell)
	require.ErrorIs(t, err, errNoEnvironment)
}

func slicesUnique(entries []string) []string {
	seen := make(map[string]bool, len(entries))
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if seen[entry] {
			continue
		}
		seen[entry] = true
		out = append(out, entry)
	}
	return out
}

// os/exec resolves a command name against the calling process's PATH and
// ignores the environment handed to the child, so the resolver has to answer
// this itself or a command only the login shell knows about never starts.
func TestLookPathSearchesTheResolvedPath(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "tool")
	require.NoError(t, os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme"), []byte("not executable"), 0o644))
	t.Setenv("PATH", "/nonexistent")

	r := NewResolver(Options{Shell: "/bin/sh", Probe: func(context.Context, string) (map[string]string, error) {
		return map[string]string{"PATH": dir}, nil
	}})

	got, err := r.LookPath(t.Context(), "tool")
	require.NoError(t, err)
	assert.Equal(t, tool, got)

	got, err = r.LookPath(t.Context(), "/usr/bin/env")
	require.NoError(t, err)
	assert.Equal(t, "/usr/bin/env", got, "a path is not a name to search for")

	_, err = r.LookPath(t.Context(), "readme")
	require.ErrorIs(t, err, exec.ErrNotFound, "a file that is not executable is not a command")
}
