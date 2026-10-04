package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/config"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/cmd/desktop/internal/devproxy"
	"github.com/colonyops/hive/internal/platform/credentials"
	"github.com/rs/zerolog"
)

const launchMarkerEnv = "HIVE_DESKTOP_LAUNCH_ENV"

// launchKeys is the staleness contract: a launch.env missing any of these is
// regenerated rather than reused, which is how a worktree written before a key
// existed opts in with no manual step. **A key added to the generated env
// belongs here too** — otherwise every existing worktree keeps a launch.env
// without it, and the feature it gates silently stays off in exactly the
// worktrees that have been around longest.
var launchKeys = []string{
	credentials.EnvKeyringService,
	settings.EnvDataDir,
	settings.EnvConfigDir,
	settings.EnvAgentWorkspacesDir,
	settings.EnvGitHubAPIBase,
	settings.EnvLogLevel,
	settings.EnvHTTPEnabled,
	settings.EnvHTTPPort,
	settings.EnvPerfEnabled,
	"WAILS_VITE_HOST",
	"WAILS_VITE_PORT",
	"WAILS_SERVER_HOST",
	"WAILS_SERVER_PORT",
	"WAILS_MCP",
	"WAILS_MCP_HOST",
	"WAILS_MCP_PORT",
	launchMarkerEnv,
}

// errLaunchEnvUnusable marks a launch.env that exists but cannot be reused —
// stale (missing a key added since it was written), corrupt, or copied from
// another worktree. Prepare and reset regenerate rather than fail on it, so a
// worktree from before a launch-key change opts in with no manual step.
var errLaunchEnvUnusable = errors.New("launch.env is unusable")

type devtools struct {
	worktree    string
	instanceDir string
	markerPath  string
	launchPath  string
	mcpPath     string
	lockPath    string
	stdout      io.Writer
	stderr      io.Writer
	pid         int
	alive       func(int) bool
	lookPath    func(string) (string, error)
	runCommand  func(string, ...string) error
	logger      zerolog.Logger
	blank       bool
	keyring     string
}

func newDevtools(worktree string, logger zerolog.Logger) *devtools {
	return newDevtoolsInstance(worktree, logger, false)
}

func newOnboardingDevtools(worktree string, logger zerolog.Logger) *devtools {
	return newDevtoolsInstance(worktree, logger, true)
}

func newDevtoolsInstance(worktree string, logger zerolog.Logger, blank bool) *devtools {
	worktree = filepath.Clean(worktree)
	instanceName := ".hive-desktop"
	launchName := "launch.env"
	lockName := ".hive-desktop.lock"
	keyring := ""
	if blank {
		instanceName = ".hive-desktop-onboarding"
		launchName = "launch.onboarding.env"
		lockName = ".hive-desktop-onboarding.lock"
		sum := sha256.Sum256([]byte(worktree))
		keyring = fmt.Sprintf("sh.hive.desktop.development.onboarding.%x", sum[:8])
	}
	mcpPath := filepath.Join(worktree, ".mcp.json")
	if blank {
		mcpPath = ""
	}
	d := &devtools{
		worktree:    worktree,
		instanceDir: filepath.Join(worktree, instanceName),
		launchPath:  filepath.Join(worktree, launchName),
		mcpPath:     mcpPath,
		lockPath:    filepath.Join(worktree, lockName),
		stdout:      os.Stdout,
		stderr:      os.Stderr,
		pid:         os.Getpid(),
		alive:       processAlive,
		lookPath:    exec.LookPath,
		logger:      logger,
		blank:       blank,
		keyring:     keyring,
	}
	d.markerPath = filepath.Join(d.instanceDir, ".instance")
	d.runCommand = func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.Stdout = d.stdout
		cmd.Stderr = d.stderr
		return cmd.Run()
	}
	return d
}

func findWorktree() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("development tooling must run inside a git worktree: %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", errors.New("git returned an empty worktree path")
	}
	return filepath.Abs(root)
}

func (d *devtools) validatePaths() error {
	root, err := filepath.Abs(d.worktree)
	if err != nil {
		return err
	}
	if filepath.Clean(d.worktree) != root {
		return fmt.Errorf("worktree path must be absolute: %s", d.worktree)
	}
	instanceName := ".hive-desktop"
	launchName := "launch.env"
	lockName := ".hive-desktop.lock"
	if d.blank {
		instanceName = ".hive-desktop-onboarding"
		launchName = "launch.onboarding.env"
		lockName = ".hive-desktop-onboarding.lock"
	}
	wantInstance := filepath.Join(root, instanceName)
	wantLaunch := filepath.Join(root, launchName)
	wantLock := filepath.Join(root, lockName)
	if d.instanceDir != wantInstance || d.launchPath != wantLaunch || d.lockPath != wantLock {
		return errors.New("development paths escaped the worktree")
	}
	paths := []string{d.instanceDir, d.launchPath, d.lockPath}
	if d.mcpPath != "" {
		if d.mcpPath != filepath.Join(root, ".mcp.json") {
			return errors.New("development paths escaped the worktree")
		}
		paths = append(paths, d.mcpPath)
	}
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink at %s", path)
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// sharedHiveDataDir is the hive data dir a regular instance shares with the
// installed app: the CLI's HIVE_DATA_DIR when the shell exports it, else the
// installed data dir. Only HIVE_DATA_DIR is read from the environment: the
// task environment can carry HIVE_DESKTOP_HIVE_DATA_DIR from an earlier
// launch.env.
func sharedHiveDataDir(installedDataDir string) (string, error) {
	dir := os.Getenv(config.EnvDataDir)
	if dir == "" {
		return installedDataDir, nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", config.EnvDataDir, err)
	}
	return abs, nil
}

// refreshHiveDataDir keeps a reused launch.env on the hive data dir the
// installed app uses now. A developer can export or change HIVE_DATA_DIR after
// the instance exists, and nothing else rewrites a valid launch.env.
func (d *devtools) refreshHiveDataDir(launch map[string]string) error {
	if d.blank {
		return nil
	}
	installed, err := installedPaths()
	if err != nil {
		return err
	}
	want, err := sharedHiveDataDir(installed.DataDir)
	if err != nil {
		return err
	}
	if launch[settings.EnvHiveDataDir] == want {
		return nil
	}
	launch[settings.EnvHiveDataDir] = want
	return writeDotenvAtomic(d.launchPath, launch)
}

func (d *devtools) prepare(fresh bool) error {
	if err := d.validatePaths(); err != nil {
		return err
	}
	if fresh {
		if err := d.teardown(); err != nil {
			return err
		}
	} else {
		launch, err := d.readLaunchIfPresent()
		switch {
		case err == nil && launch == nil:
			// No launch.env to reuse: write one over the existing instance.
		case err == nil:
			if err := d.validateInstance(); err == nil {
				if err := d.refreshHiveDataDir(launch); err != nil {
					return err
				}
				if err := d.writeMCPConfig(launch); err != nil {
					return err
				}
				d.logger.Info().Str("instance", d.instanceDir).Str("launch_env", d.launchPath).Msg("reusing desktop development environment")
				return nil
			}
		case errors.Is(err, errLaunchEnvUnusable):
			d.logger.Warn().Err(err).Msg("existing launch.env unusable; regenerating")
		default:
			return err
		}
	}

	created, err := d.ensureInstance()
	if err != nil {
		return err
	}

	dataDir := filepath.Join(d.instanceDir, "data")
	configDir := filepath.Join(d.instanceDir, "config")
	agentWorkspacesDir := filepath.Join(configDir, "workspaces")
	hiveDataDir := dataDir
	// A blank instance leaves the agent workspace root for the app to create:
	// the app seeds the Hive workspace only into a root it made itself, which
	// is what a fresh install sees.
	if !d.blank {
		sourcePaths, err := installedPaths()
		if err != nil {
			return err
		}
		if hiveDataDir, err = sharedHiveDataDir(sourcePaths.DataDir); err != nil {
			return err
		}
		if created {
			if err := d.seedData(sourcePaths.DataDir, dataDir); err != nil {
				return err
			}
			if err := d.seedConfig(sourcePaths.ConfigDir, configDir); err != nil {
				return err
			}
		}
		defaultInstalledWorkspaces := filepath.Join(sourcePaths.ConfigDir, "workspaces")
		replaceAgentWorkspaces := created && filepath.Clean(sourcePaths.AgentWorkspacesDir) != filepath.Clean(defaultInstalledWorkspaces)
		if err := d.seedAgentWorkspaces(sourcePaths.AgentWorkspacesDir, agentWorkspacesDir, replaceAgentWorkspaces); err != nil {
			return err
		}
	}

	cfg, err := settings.NewStore(filepath.Join(configDir, "settings.yaml")).Persisted()
	if err != nil {
		return fmt.Errorf("load development settings: %w", err)
	}

	vitePort, err := resolvePort(cfg.Development.Vite, 0)
	if err != nil {
		return fmt.Errorf("resolve Vite port: %w", err)
	}
	wailsPort, err := resolvePort(cfg.Development.Wails, vitePort)
	if err != nil {
		return fmt.Errorf("resolve Wails port: %w", err)
	}
	// The webhook listener boots on so an agent can push deliveries and reach
	// the agent HTTP API, which shares this port (ADR agent-http-api). Preserved across
	// prepares by the reuse path above.
	webhookPort, err := freePort(vitePort, wailsPort)
	if err != nil {
		return fmt.Errorf("resolve webhook port: %w", err)
	}
	// The Wails MCP server defaults to 9099, which two worktrees' dev apps
	// would fight over (ADR the-dev-build-compiles-in-the-wails-mcp-server-and-agents-drive-the-native-ui-through-it).
	mcpPort, err := freePort(vitePort, wailsPort, webhookPort)
	if err != nil {
		return fmt.Errorf("resolve Wails MCP port: %w", err)
	}
	// Development runs through the shared proxy by default (ADR devserver-github-proxy): the
	// address comes from the checked-in devserver config, so changing the port
	// there reaches every worktree without editing this. Opting out is setting
	// the same variable empty in the gitignored overrides.env, which mise loads
	// after launch.env.
	githubAPIBase := ""
	if !d.blank {
		githubAPIBase = devproxy.BaseURL(devproxy.ListenFromConfig(d.worktree))
	}

	env := map[string]string{
		credentials.EnvKeyringService:  d.keyring,
		settings.EnvDataDir:            dataDir,
		settings.EnvHiveDataDir:        hiveDataDir,
		settings.EnvConfigDir:          configDir,
		settings.EnvAgentWorkspacesDir: agentWorkspacesDir,
		settings.EnvGitHubAPIBase:      githubAPIBase,
		settings.EnvLogLevel:           "debug",
		settings.EnvHTTPEnabled:        "true",
		settings.EnvHTTPPort:           strconv.Itoa(webhookPort),
		settings.EnvPerfEnabled:        "true",
		"WAILS_VITE_HOST":              cfg.Development.Vite.Host,
		"WAILS_VITE_PORT":              strconv.Itoa(vitePort),
		"WAILS_SERVER_HOST":            cfg.Development.Wails.Host,
		"WAILS_SERVER_PORT":            strconv.Itoa(wailsPort),
		"WAILS_MCP":                    "1",
		"WAILS_MCP_HOST":               "127.0.0.1",
		"WAILS_MCP_PORT":               strconv.Itoa(mcpPort),
		launchMarkerEnv:                d.launchPath,
	}
	if err := writeDotenvAtomic(d.launchPath, env); err != nil {
		return err
	}
	if err := d.writeMCPConfig(env); err != nil {
		return err
	}
	verb := "reusing"
	if created || fresh {
		verb = "prepared"
	}
	d.logger.Info().
		Str("state", verb).
		Str("instance", d.instanceDir).
		Str("launch_env", d.launchPath).
		Str("vite", net.JoinHostPort(cfg.Development.Vite.Host, strconv.Itoa(vitePort))).
		Str("wails", net.JoinHostPort(cfg.Development.Wails.Host, strconv.Itoa(wailsPort))).
		Str("webhook", net.JoinHostPort("127.0.0.1", strconv.Itoa(webhookPort))).
		Str("mcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(mcpPort))).
		Msg("desktop development environment ready")
	return nil
}

func (d *devtools) reset() error {
	if err := d.validatePaths(); err != nil {
		return err
	}
	if err := d.teardown(); err != nil {
		return err
	}
	d.logger.Info().Str("instance", d.instanceDir).Str("launch_env", d.launchPath).Msg("reset desktop development environment")
	return nil
}

// teardown removes the instance and launch.env. The launch env is read even
// when unusable — it still carries the parsed ports, so a running dev server
// blocks teardown rather than having its data deleted underneath.
func (d *devtools) teardown() error {
	existing, err := d.readLaunchIfPresent()
	if err != nil && !errors.Is(err, errLaunchEnvUnusable) {
		return err
	}
	if err := d.ensureLaunchInactive(existing); err != nil {
		return err
	}
	if err := d.removeOnboardingCredentials(); err != nil {
		return err
	}
	if err := d.removeInstance(); err != nil {
		return err
	}
	if d.mcpPath != "" {
		if err := removeRegularFile(d.mcpPath); err != nil {
			return err
		}
	}
	return removeRegularFile(d.launchPath)
}

func (d *devtools) removeOnboardingCredentials() error {
	if !d.blank {
		return nil
	}
	indexPath := filepath.Join(d.instanceDir, "data", "desktop", "credentials.json")
	store := credentials.NewKeychainStoreWithService(indexPath, d.keyring)
	refs, err := store.List()
	if err != nil {
		return fmt.Errorf("list onboarding credentials: %w", err)
	}
	for _, ref := range refs {
		if err := store.Delete(ref); err != nil {
			return fmt.Errorf("delete onboarding credential %s: %w", ref, err)
		}
	}
	return nil
}

// ensureLaunchInactive keeps fresh/reset from deleting files beneath a Wails
// process launched with launch.env. A different process occupying any of the
// ports is treated conservatively as active; regenerate only after it releases
// the configured address.
func (d *devtools) ensureLaunchInactive(launch map[string]string) error {
	if launch == nil {
		return nil
	}
	for _, server := range []struct {
		name, hostKey, portKey string
	}{
		{name: "Vite", hostKey: "WAILS_VITE_HOST", portKey: "WAILS_VITE_PORT"},
		{name: "Wails", hostKey: "WAILS_SERVER_HOST", portKey: "WAILS_SERVER_PORT"},
		{name: "Wails MCP", hostKey: "WAILS_MCP_HOST", portKey: "WAILS_MCP_PORT"},
	} {
		address := net.JoinHostPort(launch[server.hostKey], launch[server.portKey])
		connection, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err != nil {
			continue
		}
		_ = connection.Close()
		return fmt.Errorf("%s development server is active at %s; stop it before fresh/reset", server.name, address)
	}
	return nil
}

func (d *devtools) ensureInstance() (bool, error) {
	if _, err := os.Lstat(d.instanceDir); errors.Is(err, fs.ErrNotExist) {
		if err := os.Mkdir(d.instanceDir, 0o755); err != nil {
			return false, fmt.Errorf("create instance root: %w", err)
		}
		if err := os.WriteFile(d.markerPath, []byte(d.worktree+"\n"), 0o600); err != nil {
			_ = os.RemoveAll(d.instanceDir)
			return false, fmt.Errorf("write instance marker: %w", err)
		}
		if err := os.MkdirAll(filepath.Join(d.instanceDir, "data", "desktop"), 0o755); err != nil {
			return false, err
		}
		if err := os.MkdirAll(filepath.Join(d.instanceDir, "config"), 0o755); err != nil {
			return false, err
		}
		return true, nil
	} else if err != nil {
		return false, err
	}
	if err := d.validateInstance(); err != nil {
		return false, err
	}
	return false, nil
}

func (d *devtools) validateInstance() error {
	if err := d.validatePaths(); err != nil {
		return err
	}
	info, err := os.Lstat(d.instanceDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", d.instanceDir)
	}
	marker, err := os.Lstat(d.markerPath)
	if err != nil {
		return fmt.Errorf("instance marker: %w", err)
	}
	if !marker.Mode().IsRegular() {
		return errors.New("instance marker is not a regular file")
	}
	owner, err := os.ReadFile(d.markerPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(owner)) != d.worktree {
		return fmt.Errorf("instance belongs to %q, not %q", strings.TrimSpace(string(owner)), d.worktree)
	}
	return nil
}

func (d *devtools) removeInstance() error {
	_, err := os.Lstat(d.instanceDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := d.validateInstance(); err != nil {
		return err
	}
	return os.RemoveAll(d.instanceDir)
}

func removeRegularFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to remove non-regular file %s", path)
	}
	return os.Remove(path)
}

// installedPaths ignores launch-time path overrides. Development always
// clones the installed app selected by bootstrap/XDG into this worktree's
// fixed .hive-desktop roots; overrides.env applies only when Wails launches.
func installedPaths() (settings.Paths, error) {
	dataDir, hadDataDir := os.LookupEnv(settings.EnvDataDir)
	configDir, hadConfigDir := os.LookupEnv(settings.EnvConfigDir)
	agentWorkspacesDir, hadAgentWorkspacesDir := os.LookupEnv(settings.EnvAgentWorkspacesDir)
	_ = os.Unsetenv(settings.EnvDataDir)
	_ = os.Unsetenv(settings.EnvConfigDir)
	_ = os.Unsetenv(settings.EnvAgentWorkspacesDir)
	defer func() {
		if hadDataDir {
			_ = os.Setenv(settings.EnvDataDir, dataDir)
		}
		if hadConfigDir {
			_ = os.Setenv(settings.EnvConfigDir, configDir)
		}
		if hadAgentWorkspacesDir {
			_ = os.Setenv(settings.EnvAgentWorkspacesDir, agentWorkspacesDir)
		}
	}()

	bootstrap, err := settings.LoadBootstrap()
	if err != nil {
		return settings.Paths{}, fmt.Errorf("load installed bootstrap: %w", err)
	}
	paths := settings.ResolvePaths(bootstrap, settings.ResolveOptions{})
	cfg, err := settings.NewStore(paths.SettingsPath).Persisted()
	if err != nil {
		return settings.Paths{}, fmt.Errorf("load installed settings: %w", err)
	}
	return settings.ResolvePaths(bootstrap, settings.ResolveOptions{AgentWorkspacesDir: cfg.AgentWorkspaces.Dir}), nil
}

// freePort allocates a free loopback port outside exclude. Rejected sockets
// are held open until one is chosen so the OS keeps handing out distinct
// ports, then released — a small time-of-check/time-of-use window remains
// before the caller binds it.
func freePort(exclude ...int) (int, error) {
	excluded := make(map[int]bool, len(exclude))
	for _, p := range exclude {
		excluded[p] = true
	}
	var open []net.Listener
	defer func() {
		for _, ln := range open {
			_ = ln.Close()
		}
	}()
	for range 41 {
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", "0"))
		if err != nil {
			return 0, fmt.Errorf("preflight port: %w", err)
		}
		open = append(open, listener)
		addr, ok := listener.Addr().(*net.TCPAddr)
		if !ok {
			return 0, fmt.Errorf("preflight port: listener address is %T, want *net.TCPAddr", listener.Addr())
		}
		if !excluded[addr.Port] {
			return addr.Port, nil
		}
	}
	return 0, errors.New("could not find a free loopback port")
}

func resolvePort(server settings.ServerSettings, excluded int) (int, error) {
	address := net.JoinHostPort(server.Host, strconv.Itoa(server.Port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return 0, fmt.Errorf("preflight %s: %w", address, err)
	}
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return 0, fmt.Errorf("preflight %s: listener address is %T, want *net.TCPAddr", address, listener.Addr())
	}
	port := addr.Port
	if err := listener.Close(); err != nil {
		return 0, fmt.Errorf("close preflight listener: %w", err)
	}
	// Framework-owned listeners cannot accept this open socket, so a small
	// time-of-check/time-of-use race remains between preparation and startup.
	if server.Port == 0 && port == excluded {
		return resolvePort(server, excluded)
	}
	if server.Port != 0 && port == excluded {
		return 0, fmt.Errorf("configured port %d is assigned to both development servers", port)
	}
	return port, nil
}

func (d *devtools) seedData(source, destination string) error {
	if err := os.MkdirAll(filepath.Join(destination, "desktop"), 0o755); err != nil {
		return err
	}
	// hive.db is not seeded: dev points HIVE_DESKTOP_HIVE_DATA_DIR at the
	// installed hive data dir, so the isolated copy would never be opened.
	if err := d.copyDatabase(filepath.Join(source, "desktop", "desktop-pipeline.db"), filepath.Join(destination, "desktop", "desktop-pipeline.db")); err != nil {
		return err
	}
	files, _ := filepath.Glob(filepath.Join(source, "desktop", "*.json"))
	for _, sourceFile := range files {
		if err := copyFile(sourceFile, filepath.Join(destination, "desktop", filepath.Base(sourceFile))); err != nil {
			return err
		}
	}
	return nil
}

func (d *devtools) copyDatabase(source, destination string) error {
	if _, err := os.Stat(source); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if sqlite, err := d.lookPath("sqlite3"); err == nil {
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		_ = os.Remove(destination)
		escaped := strings.ReplaceAll(destination, "'", "''")
		if err := d.runCommand(sqlite, "-cmd", ".timeout 5000", source, "VACUUM INTO '"+escaped+"'"); err == nil {
			return nil
		} else {
			d.logger.Warn().Err(err).Str("source", source).Msg("SQLite snapshot failed; falling back to file copy")
		}
		_ = os.Remove(destination)
	}
	if err := copyFile(source, destination); err != nil {
		return err
	}
	for _, extension := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(source + extension); err == nil {
			if err := copyFile(source+extension, destination+extension); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *devtools) seedConfig(source, destination string) error {
	if _, err := os.Stat(source); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := copyTreeMaterialized(source, destination, make(map[string]bool)); err != nil {
		return fmt.Errorf("copy installed config: %w", err)
	}
	_ = os.Remove(filepath.Join(destination, "bootstrap.yaml"))
	// The settings schema intentionally broke; do not seed an old flat file.
	_ = os.Remove(filepath.Join(destination, "settings.yaml"))
	return nil
}

func (d *devtools) seedAgentWorkspaces(source, destination string, replace bool) error {
	info, err := os.Lstat(destination)
	if err == nil && !replace && info.IsDir() {
		return nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err == nil {
		if err := os.RemoveAll(destination); err != nil {
			return err
		}
	}
	if _, err := os.Stat(source); errors.Is(err, fs.ErrNotExist) {
		return os.MkdirAll(destination, 0o755)
	} else if err != nil {
		return err
	}
	if err := copyTreeMaterialized(source, destination, make(map[string]bool)); err != nil {
		return fmt.Errorf("copy installed agent workspaces: %w", err)
	}
	return nil
}

func copyTreeMaterialized(source, destination string, stack map[string]bool) error {
	realSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	realSource, err = filepath.Abs(realSource)
	if err != nil {
		return err
	}
	if stack[realSource] {
		return fmt.Errorf("symlink cycle at %s", source)
	}
	info, err := os.Stat(realSource)
	if err != nil {
		return err
	}
	if info.IsDir() {
		stack[realSource] = true
		defer delete(stack, realSource)
		if err := os.MkdirAll(destination, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(realSource)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyTreeMaterialized(filepath.Join(realSource, entry.Name()), filepath.Join(destination, entry.Name()), stack); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported config file type at %s", source)
	}
	return copyFile(realSource, destination)
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func (d *devtools) readLaunchIfPresent() (map[string]string, error) {
	data, err := os.ReadFile(d.launchPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	env, err := parseDotenv(data)
	if err != nil {
		return nil, fmt.Errorf("%w: parse %s: %w", errLaunchEnvUnusable, d.launchPath, err)
	}
	// The parsed map is returned even for an unusable file, so callers can still
	// read its ports for the active-server check before regenerating it.
	if env[launchMarkerEnv] != d.launchPath {
		return env, fmt.Errorf("%w: %s is not owned by this worktree", errLaunchEnvUnusable, d.launchPath)
	}
	for _, key := range launchKeys {
		if _, ok := env[key]; !ok {
			return env, fmt.Errorf("%w: %s is missing %s", errLaunchEnvUnusable, d.launchPath, key)
		}
	}
	return env, nil
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
