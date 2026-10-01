package main

import (
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/cmd/desktop/internal/devproxy"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func testDevtools(t *testing.T) (*devtools, string, string) {
	t.Helper()
	root := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "data-home")
	configHome := filepath.Join(t.TempDir(), "config-home")
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv(settings.EnvDataDir, "")
	t.Setenv(settings.EnvConfigDir, "")
	tools := newDevtools(root, zerolog.Nop())
	tools.stderr = &bytes.Buffer{}
	tools.stdout = &bytes.Buffer{}
	tools.lookPath = func(string) (string, error) { return "", errors.New("not installed") }
	return tools, filepath.Join(dataHome, "hive"), filepath.Join(configHome, "hive", "desktop")
}

func TestPrepareReuseFreshAndReset(t *testing.T) {
	tools, sourceData, sourceConfig := testDevtools(t)
	require.NoError(t, os.MkdirAll(filepath.Join(sourceData, "desktop"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceData, "hive.db"), []byte("db"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sourceData, "desktop", "desktop-pipeline.db"), []byte("db"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(sourceConfig, "flows"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceConfig, "actions.yml"), []byte("actions"), 0o600))

	require.NoError(t, tools.withLock(func() error { return tools.prepare(false) }))
	launch, err := tools.readLaunchIfPresent()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tools.instanceDir, "data"), launch[settings.EnvDataDir])
	assert.Equal(t, filepath.Join(tools.instanceDir, "config"), launch[settings.EnvConfigDir])
	assert.Equal(t, filepath.Join(tools.instanceDir, "config", "workspaces"), launch[settings.EnvAgentWorkspacesDir])
	assert.DirExists(t, launch[settings.EnvAgentWorkspacesDir])
	vite, err := strconv.Atoi(launch["WAILS_VITE_PORT"])
	require.NoError(t, err)
	wails, err := strconv.Atoi(launch["WAILS_SERVER_PORT"])
	require.NoError(t, err)
	assert.NotZero(t, vite)
	assert.NotZero(t, wails)
	assert.NotEqual(t, vite, wails)
	// The loopback HTTP server (webhook listener + agent API) boots on a
	// distinct allocated port (Item 3).
	assert.Equal(t, "true", launch[settings.EnvHTTPEnabled])
	httpPort, err := strconv.Atoi(launch[settings.EnvHTTPPort])
	require.NoError(t, err)
	assert.NotZero(t, httpPort)
	assert.NotEqual(t, vite, httpPort)
	assert.NotEqual(t, wails, httpPort)
	// hive.db is not seeded; dev points at the installed hive data dir instead.
	assert.NoFileExists(t, filepath.Join(tools.instanceDir, "data", "hive.db"))
	assert.Equal(t, sourceData, launch[settings.EnvHiveDataDir])
	assert.FileExists(t, filepath.Join(tools.instanceDir, "data", "desktop", "desktop-pipeline.db"))
	assert.FileExists(t, filepath.Join(tools.instanceDir, "config", "actions.yml"))

	sentinel := filepath.Join(tools.instanceDir, "data", "keep")
	require.NoError(t, os.WriteFile(sentinel, []byte("keep"), 0o600))
	require.NoError(t, tools.withLock(func() error { return tools.prepare(false) }))
	assert.FileExists(t, sentinel)

	// Ambient launch paths must never become the source of a fresh snapshot.
	// Developer overrides are loaded only by the dev mise task.
	t.Setenv(settings.EnvDataDir, launch[settings.EnvDataDir])
	t.Setenv(settings.EnvConfigDir, launch[settings.EnvConfigDir])
	require.NoError(t, os.WriteFile(filepath.Join(sourceData, "desktop", "desktop-pipeline.db"), []byte("new-db"), 0o600))
	require.NoError(t, tools.withLock(func() error { return tools.prepare(true) }))
	assert.NoFileExists(t, sentinel)
	assert.FileExists(t, tools.launchPath)
	copiedDB, err := os.ReadFile(filepath.Join(tools.instanceDir, "data", "desktop", "desktop-pipeline.db"))
	require.NoError(t, err)
	assert.Equal(t, "new-db", string(copiedDB))

	require.NoError(t, tools.withLock(tools.reset))
	assert.NoDirExists(t, tools.instanceDir)
	assert.NoFileExists(t, tools.launchPath)
}

func TestPrepareOnboardingUsesBlankIsolatedState(t *testing.T) {
	keyring.MockInit()
	normal, sourceData, sourceConfig := testDevtools(t)
	require.NoError(t, os.MkdirAll(filepath.Join(sourceData, "desktop"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceData, "desktop", "desktop-pipeline.db"), []byte("installed"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(sourceConfig, "flows"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceConfig, "flows", "work.yaml"), []byte("installed"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sourceConfig, "actions.yml"), []byte("installed"), 0o600))
	require.NoError(t, normal.withLock(func() error { return normal.prepare(false) }))
	normalSentinel := filepath.Join(normal.instanceDir, "keep")
	require.NoError(t, os.WriteFile(normalSentinel, []byte("keep"), 0o600))

	tools := newOnboardingDevtools(normal.worktree, zerolog.Nop())
	tools.stderr = &bytes.Buffer{}
	tools.stdout = &bytes.Buffer{}
	require.NoError(t, tools.withLock(func() error { return tools.prepare(true) }))

	launch, err := tools.readLaunchIfPresent()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tools.instanceDir, "data"), launch[settings.EnvDataDir])
	assert.Equal(t, filepath.Join(tools.instanceDir, "data"), launch[settings.EnvHiveDataDir])
	assert.Equal(t, filepath.Join(tools.instanceDir, "config"), launch[settings.EnvConfigDir])
	assert.Empty(t, launch[settings.EnvGitHubAPIBase])
	assert.NotEmpty(t, launch[credentials.EnvKeyringService])
	assert.FileExists(t, normalSentinel)
	assert.NoFileExists(t, filepath.Join(tools.instanceDir, "data", "desktop", "desktop-pipeline.db"))
	assert.NoFileExists(t, filepath.Join(tools.instanceDir, "config", "flows", "work.yaml"))
	assert.NoFileExists(t, filepath.Join(tools.instanceDir, "config", "actions.yml"))
	assert.NoDirExists(t, filepath.Join(tools.instanceDir, "config", "workspaces"))

	ref := credentials.Ref{Provider: "github", Account: "octocat"}
	store := credentials.NewKeychainStoreWithService(
		filepath.Join(tools.instanceDir, "data", "desktop", "credentials.json"),
		launch[credentials.EnvKeyringService],
	)
	require.NoError(t, store.Set(ref, "token-value"))
	require.NoError(t, tools.withLock(func() error { return tools.prepare(true) }))
	_, err = store.Get(ref)
	require.ErrorIs(t, err, credentials.ErrNotFound)
	assert.FileExists(t, normalSentinel)
}

func TestPrepareMaterializesConfigSymlinks(t *testing.T) {
	tools, _, sourceConfig := testDevtools(t)
	external := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(external, "flow.yaml"), []byte("flow"), 0o600))
	require.NoError(t, os.MkdirAll(sourceConfig, 0o755))
	require.NoError(t, os.Symlink(external, filepath.Join(sourceConfig, "flows")))

	require.NoError(t, tools.withLock(func() error { return tools.prepare(false) }))
	copied := filepath.Join(tools.instanceDir, "config", "flows")
	info, err := os.Lstat(copied)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Zero(t, info.Mode()&os.ModeSymlink)
	assert.FileExists(t, filepath.Join(copied, "flow.yaml"))
}

func TestPrepareCopiesConfiguredAgentWorkspaces(t *testing.T) {
	tools, _, sourceConfig := testDevtools(t)
	external := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(external, "personal"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(external, "personal", "agent-workspace.yaml"), []byte("workspace"), 0o600))
	require.NoError(t, os.MkdirAll(sourceConfig, 0o755))
	settingsYAML := "version: 1\nagent_workspaces:\n  dir: " + external + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(sourceConfig, "settings.yaml"), []byte(settingsYAML), 0o600))

	require.NoError(t, tools.withLock(func() error { return tools.prepare(false) }))
	launch, err := tools.readLaunchIfPresent()
	require.NoError(t, err)
	copied := filepath.Join(tools.instanceDir, "config", "workspaces")
	assert.Equal(t, copied, launch[settings.EnvAgentWorkspacesDir])
	assert.FileExists(t, filepath.Join(copied, "personal", "agent-workspace.yaml"))

	t.Setenv(settings.EnvAgentWorkspacesDir, filepath.Join(t.TempDir(), "ambient"))
	require.NoError(t, tools.withLock(func() error { return tools.prepare(true) }))
	assert.FileExists(t, filepath.Join(copied, "personal", "agent-workspace.yaml"))
}

func TestFreshAndResetRefuseActiveLaunch(t *testing.T) {
	tools, _, _ := testDevtools(t)
	require.NoError(t, tools.withLock(func() error { return tools.prepare(false) }))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })
	launch, err := tools.readLaunchIfPresent()
	require.NoError(t, err)
	launch["WAILS_SERVER_HOST"] = "127.0.0.1"
	addr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok, "listener address should be *net.TCPAddr")
	launch["WAILS_SERVER_PORT"] = strconv.Itoa(addr.Port)
	require.NoError(t, writeDotenvAtomic(tools.launchPath, launch))

	err = tools.withLock(func() error { return tools.prepare(true) })
	require.ErrorContains(t, err, "Wails development server is active")
	err = tools.withLock(tools.reset)
	require.ErrorContains(t, err, "Wails development server is active")
	assert.DirExists(t, tools.instanceDir)
	assert.FileExists(t, tools.launchPath)
}

func TestInstanceSafetyRejectsWrongMarkerAndSymlink(t *testing.T) {
	tools, _, _ := testDevtools(t)
	require.NoError(t, os.Mkdir(tools.instanceDir, 0o755))
	require.NoError(t, os.WriteFile(tools.markerPath, []byte("someone-else\n"), 0o600))
	require.ErrorContains(t, tools.removeInstance(), "belongs to")
	assert.DirExists(t, tools.instanceDir)

	require.NoError(t, os.RemoveAll(tools.instanceDir))
	require.NoError(t, os.Symlink(t.TempDir(), tools.instanceDir))
	assert.ErrorContains(t, tools.validatePaths(), "refusing symlink")
}

func TestDotenvRoundTripAndAtomicReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launch.env")
	want := map[string]string{"ALPHA": "space and \"quote\"", "PATH_VALUE": "/tmp/a b", "UNICODE": "蜂"}
	require.NoError(t, writeDotenvAtomic(path, want))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	got, err := parseDotenv(data)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.NoFileExists(t, filepath.Join(filepath.Dir(path), ".launch-stale.env"))

	require.NoError(t, writeDotenvAtomic(path, map[string]string{"ALPHA": "second"}))
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	got, err = parseDotenv(data)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"ALPHA": "second"}, got)
}

func TestParseDotenvRejectsMalformedAndDuplicateEntries(t *testing.T) {
	_, err := parseDotenv([]byte("NO_QUOTES=value\n"))
	require.Error(t, err)
	_, err = parseDotenv([]byte("A=\"one\"\nA=\"two\"\n"))
	assert.ErrorContains(t, err, "duplicate")
}

func TestLockRejectsActiveAndRecoversStaleOwner(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".hive-desktop.lock")
	require.NoError(t, os.Mkdir(path, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "owner"), []byte(root+"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(path, "pid"), []byte("123\n"), 0o600))

	active := &instanceLock{path: path, worktree: root, pid: 456, alive: func(pid int) bool { return pid == 123 }, logger: zerolog.Nop()}
	require.ErrorContains(t, active.acquire(), "active")

	stale := &instanceLock{path: path, worktree: root, pid: 456, alive: func(int) bool { return false }, logger: zerolog.Nop()}
	require.NoError(t, stale.acquire())
	assert.True(t, stale.owned)
	pid, err := os.ReadFile(filepath.Join(path, "pid"))
	require.NoError(t, err)
	assert.Equal(t, "456", strings.TrimSpace(string(pid)))
	stale.release()
	assert.NoDirExists(t, path)
}

func TestLockRejectsDifferentOwner(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".hive-desktop.lock")
	require.NoError(t, os.Mkdir(path, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "owner"), []byte("other\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(path, "pid"), []byte("123\n"), 0o600))
	lock := &instanceLock{path: path, worktree: root, pid: 456, alive: func(int) bool { return false }, logger: zerolog.Nop()}
	assert.ErrorContains(t, lock.acquire(), "belongs to")
}

// devproxy duplicates the env name rather than importing cmd/desktop/internal/app/settings,
// which keeps cmd/desktop/devserver free of any dependency on app packages. devtools
// imports both, so it is the one place the two spellings can be compared.
func TestDevproxyEnvNameMatchesSettings(t *testing.T) {
	assert.Equal(t, settings.EnvGitHubAPIBase, devproxy.EnvAPIBase)
}

// A launch.env from before these keys were added must not wedge prepare: it is
// regenerated rather than reported as an error.
func TestPrepareRegeneratesStaleLaunchEnv(t *testing.T) {
	tools, _, _ := testDevtools(t)
	require.NoError(t, tools.prepare(false))

	stale := map[string]string{
		settings.EnvDataDir:       filepath.Join(tools.instanceDir, "data"),
		settings.EnvConfigDir:     filepath.Join(tools.instanceDir, "config"),
		settings.EnvGitHubAPIBase: "http://127.0.0.1:7777",
		"WAILS_VITE_HOST":         "127.0.0.1",
		"WAILS_VITE_PORT":         "1",
		"WAILS_SERVER_HOST":       "127.0.0.1",
		"WAILS_SERVER_PORT":       "2",
		launchMarkerEnv:           tools.launchPath,
	}
	require.NoError(t, writeDotenvAtomic(tools.launchPath, stale))
	_, err := tools.readLaunchIfPresent()
	require.ErrorIs(t, err, errLaunchEnvUnusable, "the stale file is detected as unusable")

	require.NoError(t, tools.prepare(false), "prepare regenerates instead of failing")
	launch, err := tools.readLaunchIfPresent()
	require.NoError(t, err)
	assert.Equal(t, "true", launch[settings.EnvHTTPEnabled])
	assert.Equal(t, filepath.Join(tools.instanceDir, "config", "workspaces"), launch[settings.EnvAgentWorkspacesDir])
}

// Development is proxied by default (ADR devserver-github-proxy): prepare must write the API base
// into launch.env so a worktree opts in with no manual step, and it must take
// the address from the checked-in devserver config.
func TestPrepareWritesProxyAPIBase(t *testing.T) {
	tools, _, _ := testDevtools(t)
	require.NoError(t, tools.prepare(false))

	launch, err := tools.readLaunchIfPresent()
	require.NoError(t, err)
	assert.Equal(t,
		devproxy.BaseURL(devproxy.ListenFromConfig(tools.worktree)),
		launch[settings.EnvGitHubAPIBase])
}
