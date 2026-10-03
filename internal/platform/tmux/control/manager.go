package tmuxcc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/platform/observe"
)

// minMajor/minMinor is the control-mode floor: 3.2 is where pause mode and
// subscriptions landed, and it is the version this client is written against.
const (
	minMajor = 3
	minMinor = 2
)

// ManagerOptions configures the client set.
type ManagerOptions struct {
	Logger      zerolog.Logger
	BufferBytes int

	// Binary answers which tmux to exec, and is consulted on every failed
	// availability probe rather than once at startup, so installing tmux does not
	// need a relaunch. nil means $PATH.
	Binary func() (string, error)

	// Environ answers the environment tmux is spawned with — execenv's resolved
	// PATH in the app, whatever a test hands it otherwise. nil means this
	// process's own. It is the same hook ptyterm.ManagerOptions carries, held
	// separately because the two backends share no interface (ADR ephemeral-popup-terminals).
	Environ func(context.Context) []string

	versionProbe func(context.Context, string) (string, error)
	newProcess   func(Options) process
	onEmit       func()
	runTmux      func(context.Context, string, []string, ...string) ([]string, error)
}

// managedClient carries the generation its registration was made under, so a
// teardown that fires late cannot evict the client that replaced it.
type managedClient struct {
	gen    uint64
	client *Client
	cancel context.CancelFunc
}

// Manager owns one control-mode client per session slug.
type Manager struct {
	log         zerolog.Logger
	locate      func() (string, error)
	environ     func(context.Context) []string
	probe       func(context.Context, string) (string, error)
	newProcess  func(Options) process
	onEmit      func()
	run         func(context.Context, string, []string, ...string) ([]string, error)
	bufferBytes int

	// The app-lifetime context lives in this closure rather than in a field:
	// every client context descends from it, and Stop cancels the lot.
	derive    func() (context.Context, context.CancelFunc)
	cancelAll context.CancelFunc

	attachMu sync.Mutex

	mu      sync.Mutex
	clients map[string]*managedClient
	gen     uint64
	stopped bool
	probed  bool
	binary  string

	stopOnce sync.Once
}

// NewManager builds the client set. ctx is the app's lifetime context: every
// tmux client's goroutines descend from it.
func NewManager(ctx context.Context, opts ManagerOptions) *Manager {
	lifetime, cancel := context.WithCancel(ctx)

	m := &Manager{
		log:         opts.Logger,
		locate:      opts.Binary,
		environ:     opts.Environ,
		probe:       opts.versionProbe,
		newProcess:  opts.newProcess,
		onEmit:      opts.onEmit,
		run:         opts.runTmux,
		bufferBytes: opts.BufferBytes,
		cancelAll:   cancel,
		derive:      func() (context.Context, context.CancelFunc) { return context.WithCancel(lifetime) },
		clients:     map[string]*managedClient{},
	}
	if m.locate == nil {
		m.locate = func() (string, error) { return defaultBinary, nil }
	}
	if m.environ == nil {
		m.environ = func(context.Context) []string { return os.Environ() }
	}
	if m.probe == nil {
		m.probe = tmuxVersion
	}
	if m.run == nil {
		m.run = runTmux
	}
	return m
}

// Available reports nil when a usable tmux (>= 3.2) is present on a supported
// build and platform, else ErrUnavailable. A successful probe is cached along
// with the binary it ran; a failing one is retried, so installing tmux does not
// require a restart.
func (m *Manager) Available(ctx context.Context) error {
	if !platformSupported() {
		return ErrUnavailable
	}

	m.mu.Lock()
	probed := m.probed
	m.mu.Unlock()
	if probed {
		return nil
	}

	binary, err := m.locate()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	raw, err := m.probe(ctx, binary)
	if err != nil {
		return fmt.Errorf("%w: %s not usable: %w", ErrUnavailable, binary, err)
	}
	if !versionAtLeast(raw, minMajor, minMinor) {
		return fmt.Errorf("%w: %s is %s, older than %d.%d", ErrUnavailable, binary, strings.TrimSpace(raw), minMajor, minMinor)
	}

	m.mu.Lock()
	m.probed = true
	m.binary = binary
	m.mu.Unlock()
	m.log.Info().Ctx(ctx).Str("tmux", binary).Str("version", strings.TrimSpace(raw)).Msg("tmux control mode available")
	return nil
}

// Attach opens, or returns the windows of, the client for slug, and either way
// leaves a first paint on its stream: a caller attaches to render, and the one
// re-attaching over a transport that dropped has nothing else to render from.
// A second attach for a live slug therefore reuses the client and repaints it,
// keeping the size it already voted — the vote belongs to a fresh attach. After
// a client exits its slug is free again and a re-attach starts fresh. cols/rows
// of 0x0 attach unsized — see Options.unsized.
func (m *Manager) Attach(ctx context.Context, slug string, cols, rows int) ([]Window, error) {
	if err := m.Available(ctx); err != nil {
		return nil, err
	}
	if err := validateAttachSize(cols, rows); err != nil {
		return nil, err
	}
	if slug == "" {
		return nil, fmt.Errorf("%w: empty slug", ErrNotAttached)
	}

	m.attachMu.Lock()
	defer m.attachMu.Unlock()

	if mc, ok := m.managed(slug); ok {
		if _, dead := mc.client.exited(); !dead {
			// A live client keeps the grid its original surface voted, which a
			// re-attach from a differently-sized pane must not inherit: repaint
			// at the stale size and the snapshot tears the moment the new
			// surface's own vote lands — fullscreen being the worst case.
			// Renegotiating first paints the repaint at the caller's size; 0x0
			// attaches unsized and keeps whatever stands.
			if cols > 0 && rows > 0 {
				if err := mc.client.Renegotiate(ctx, cols, rows); err != nil {
					return nil, err
				}
			}
			if err := mc.client.Repaint(ctx); err != nil {
				return nil, err
			}
			return mc.client.Windows(), nil
		}
		m.remove(slug, mc.gen)
	}

	m.mu.Lock()
	stopped := m.stopped
	binary := m.binary
	m.gen++
	gen := m.gen
	m.mu.Unlock()
	if stopped {
		return nil, ErrUnavailable
	}

	lifetime, cancel := m.derive()
	client, err := Attach(ctx, lifetime, Options{
		Slug:        slug,
		Cols:        cols,
		Rows:        rows,
		Binary:      binary,
		Environ:     m.environ(ctx),
		BufferBytes: m.bufferBytes,
		Logger:      m.log,
		OnExit:      func(slug, _ string) { m.remove(slug, gen) },
		newProcess:  m.newProcess,
		onEmit:      m.onEmit,
	})
	if err != nil {
		cancel()
		return nil, err
	}

	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		_ = client.Close(ctx)
		cancel()
		return nil, ErrUnavailable
	}
	// Teardown can fire before Attach returns, and its OnExit then has nothing
	// to remove. Reading the flag under the lock the entry is stored under
	// leaves no gap: either it is already set and we never register, or OnExit
	// is still waiting on this lock and evicts what we just stored.
	if reason, dead := client.exited(); dead {
		m.mu.Unlock()
		cancel()
		return nil, fmt.Errorf("%w: %s exited during attach: %s", ErrNotAttached, slug, reason)
	}
	m.clients[slug] = &managedClient{gen: gen, client: client, cancel: cancel}
	m.mu.Unlock()

	return client.Windows(), nil
}

func (m *Manager) Client(slug string) (*Client, bool) {
	mc, ok := m.managed(slug)
	if !ok {
		return nil, false
	}
	return mc.client, true
}

func (m *Manager) managed(slug string) (*managedClient, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mc, ok := m.clients[slug]
	return mc, ok
}

// Subscribe returns slug's event channel and unsubscribe func. There is one
// active subscriber per slug; a new call closes the previous channel. Events
// buffered before the call — first paint included — replay on the new one.
func (m *Manager) Subscribe(slug string) (<-chan Event, func(), error) {
	c, ok := m.Client(slug)
	if !ok {
		return nil, nil, fmt.Errorf("%w: %s", ErrNotAttached, slug)
	}
	ch, unsubscribe := c.Subscribe()
	return ch, unsubscribe, nil
}

// RenameSession renames a live tmux session when one exists.
func (m *Manager) RenameSession(ctx context.Context, from, to string) error {
	_, err := m.RenameSessionIfPresent(ctx, from, to)
	return err
}

// RenameSessionIfPresent renames the live tmux session named from to to and
// reports whether it changed tmux. It drops the control client registered under
// the old slug so nothing keeps addressing a name tmux no longer answers to.
//
// A session hive knows about does not have to have a tmux session behind it --
// it may never have been spawned, or its tmux server may have been restarted --
// so an absent one is success, not an error. Existence is probed with
// has-session rather than inferred from rename-session's stderr, because the
// alternative is matching on tmux's message text.
func (m *Manager) RenameSessionIfPresent(ctx context.Context, from, to string) (bool, error) {
	if from == "" || to == "" {
		return false, fmt.Errorf("%w: empty slug", ErrInvalidName)
	}
	if from == to {
		return false, nil
	}
	// tmux being unavailable means there is no live session to keep in step:
	// the rename is a store-only concern and must not be blocked by it.
	if err := m.Available(ctx); err != nil {
		return false, nil
	}
	if _, err := m.oneShot(ctx, "has-session", "-t", from); err != nil {
		return false, nil
	}
	if _, err := m.oneShot(ctx, "rename-session", "-t", from, to); err != nil {
		return false, fmt.Errorf("tmuxcc: rename session %s to %s: %w", from, to, err)
	}
	if mc, ok := m.managed(from); ok {
		_ = mc.client.Close(ctx)
		m.remove(from, mc.gen)
	}
	return true, nil
}

// NewWindow creates a window in a session this app holds no control client for,
// and returns its id. An attached client has its own NewWindow; this is for the
// row a user pressed + on before anything attached to it.
//
// The directory is spelled out because tmux resolves an unset start-directory
// against the *client* running the command — and a one-shot command client is
// this process, so the window would open in the app's working directory. It is
// the active pane's rather than the session's for the reason CurrentPath
// explains.
func (m *Manager) NewWindow(ctx context.Context, slug string) (string, error) {
	exists, err := m.HasSession(ctx, slug)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("%w: %s is not running", ErrNotAttached, slug)
	}
	lines, err := m.oneShot(ctx, "new-window", "-t", slug, "-c", currentPathFormat, "-P", "-F", "#{window_id}")
	if err != nil {
		return "", fmt.Errorf("tmuxcc: new window in %s: %w", slug, err)
	}
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return "", fmt.Errorf("tmuxcc: new-window returned no window id")
	}
	return strings.TrimSpace(lines[0]), nil
}

// CurrentPath answers where a session's active pane is: the directory a prompt
// in it would print, which follows a cd where the session's own start directory
// does not. It is what "here" means for anything opened from a terminal on
// screen — a new window, or a launcher with no directory of its own.
//
// It is a one-shot whether or not this app holds a control client, because the
// answer is tmux's session state and asking over the control stream would cost
// a round trip to reach the same pane.
func (m *Manager) CurrentPath(ctx context.Context, slug string) (string, error) {
	exists, err := m.HasSession(ctx, slug)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("%w: %s is not running", ErrNotAttached, slug)
	}
	lines, err := m.oneShot(ctx, "display-message", "-p", "-t", slug, currentPathFormat)
	if err != nil {
		return "", fmt.Errorf("tmuxcc: current path of %s: %w", slug, err)
	}
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return "", fmt.Errorf("tmuxcc: display-message returned no path for %s", slug)
	}
	return strings.TrimSpace(lines[0]), nil
}

// HasSession reports whether tmux is running a session named slug. Attach and
// start both ask before doing anything, so "there is no session" is a probe's
// answer rather than a reading of an attach failure's message. A live client is
// proof enough, which keeps a warm re-attach free of the round trip.
func (m *Manager) HasSession(ctx context.Context, slug string) (bool, error) {
	if slug == "" {
		return false, fmt.Errorf("%w: empty slug", ErrNotAttached)
	}
	if mc, ok := m.managed(slug); ok {
		if _, dead := mc.client.exited(); !dead {
			return true, nil
		}
	}
	if err := m.Available(ctx); err != nil {
		return false, err
	}
	_, err := m.oneShot(ctx, "has-session", "-t", slug)
	return err == nil, nil
}

// KillSession kills the tmux session named slug and reports whether there was
// one to kill; a slug tmux is not running is success with nothing done, the same
// tolerance RenameSession has. The control client goes with it, synchronously:
// the child would exit on its own moments later, and a caller that re-attaches
// in between must not be handed the dying client's window set.
func (m *Manager) KillSession(ctx context.Context, slug string) (bool, error) {
	exists, err := m.HasSession(ctx, slug)
	if err != nil || !exists {
		return false, err
	}
	if mc, ok := m.managed(slug); ok {
		_ = mc.client.Close(ctx)
		m.remove(slug, mc.gen)
	}
	if _, err := m.oneShot(ctx, "kill-session", "-t", slug); err != nil {
		return false, fmt.Errorf("tmuxcc: kill session %s: %w", slug, err)
	}
	return true, nil
}

// IndexedWindow identifies a tmux window by both its stable id and mutable index.
type IndexedWindow struct {
	ID    string
	Index string
	Name  string
}

const indexedWindowsFormat = "#{session_name}|||#{window_index}|||#{window_id}|||#{window_name}"

// ListIndexedWindows answers window identities for every requested slug in one
// tmux call. Status integrations report window indices, while terminal clients
// address stable window ids, so this is the seam that joins those views.
func (m *Manager) ListIndexedWindows(ctx context.Context, slugs []string) (map[string][]IndexedWindow, error) {
	wanted := make(map[string]struct{}, len(slugs))
	for _, slug := range slugs {
		if slug != "" {
			wanted[slug] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil, nil
	}
	if err := m.Available(ctx); err != nil {
		return nil, err
	}

	windows := make(map[string][]IndexedWindow, len(wanted))
	lines, err := m.oneShot(ctx, "list-windows", "-a", "-F", indexedWindowsFormat)
	if err != nil {
		// tmux uses exit 1 for both an absent server and real command failures;
		// stderr is the only distinction it exposes.
		if strings.Contains(err.Error(), "no server running") {
			return windows, nil
		}
		return nil, fmt.Errorf("tmuxcc: list indexed windows: %w", err)
	}
	for _, line := range lines {
		parts := strings.SplitN(line, "|||", 4)
		if len(parts) != 4 {
			continue
		}
		if _, ok := wanted[parts[0]]; !ok || !validWindowID(parts[2]) {
			continue
		}
		windows[parts[0]] = append(windows[parts[0]], IndexedWindow{
			ID:    parts[2],
			Index: parts[1],
			Name:  parts[3],
		})
	}
	return windows, nil
}

// ListAllWindows answers the window sets of every slug in one tmux call.
//
// Asking per slug cost two spawns for each unattached session — a has-session
// probe and a list-windows — so a sidebar sweep spawned twice as many processes
// as it had sessions, all at once. `list-windows -a` is one spawn for the whole
// server, and a slug it has no session for is simply absent from the output,
// which is the answer has-session was being paid for.
//
// Attached slugs still answer from their live client, as ListWindows has them:
// it is authoritative, already in memory, and carries the sizes tmux settled on.
func (m *Manager) ListAllWindows(ctx context.Context, slugs []string) (map[string][]Window, error) {
	wanted := make(map[string]struct{}, len(slugs))
	for _, slug := range slugs {
		if slug != "" {
			wanted[slug] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil, nil
	}
	if err := m.Available(ctx); err != nil {
		return nil, err
	}
	windows := make(map[string][]Window, len(wanted))
	// A failure here is a server with no sessions on it, which is no windows
	// rather than an error — the same tolerance ListWindows extends to a slug
	// tmux has never heard of.
	if lines, err := m.oneShot(ctx, "list-windows", "-a", "-F", sessionWindowsFormat); err == nil {
		for _, line := range lines {
			slug, row, ok := strings.Cut(line, " ")
			if !ok {
				continue
			}
			if _, want := wanted[slug]; !want {
				continue
			}
			w, ok := parseWindowLine(row)
			if !ok {
				m.log.Warn().Str("line", line).Msg("unparseable list-windows row")
				continue
			}
			windows[slug] = append(windows[slug], w)
		}
	}
	for slug := range wanted {
		if mc, ok := m.managed(slug); ok {
			if _, dead := mc.client.exited(); !dead {
				windows[slug] = mc.client.Windows()
			}
		}
	}
	return windows, nil
}

// oneShot runs a one-shot tmux command with the binary the availability probe
// resolved (ADR tmux-discovery) — never a bare "tmux", which a desktop launch may not
// have on $PATH. Callers go through Available first, which is what sets it.
//
// The resolved environment goes with it because a pane inherits the client
// that created it, so this is what puts an agent binary on NewSession's PATH
// (ADR tmux-runs-in-the-resolved-environment).
//
// The span is named for the subcommand, which every call site passes as a
// literal, so the name stays a bounded search key. It records no error status:
// a non-zero exit is how has-session answers "no" and how kill-session reports
// nothing to kill, so only the caller knows whether this failed.
func (m *Manager) oneShot(ctx context.Context, args ...string) ([]string, error) {
	ctx, span := observe.StartConditionalSpan(ctx, tracer, "tmux."+args[0])
	defer span.End()

	m.mu.Lock()
	binary := m.binary
	m.mu.Unlock()
	return m.run(ctx, binary, m.environ(ctx), args...)
}

// Detach closes slug's client. Unknown slugs are a no-op.
func (m *Manager) Detach(ctx context.Context, slug string) error {
	mc, ok := m.managed(slug)
	if !ok {
		return nil
	}
	err := mc.client.Close(ctx)
	m.remove(slug, mc.gen)
	return err
}

// Stop closes every client, joins their readers, and cancels the app-lifetime
// context. Idempotent.
//
// Clients close concurrently and the join is bounded by ctx, because closing
// one is not: teardown waits on the reader, the command worker and the tmux
// child with no deadline of its own, and the detach it sends first writes to a
// pipe that nothing guarantees is being read. Serially and unbounded, a single
// client that will not come down holds App.Close — and therefore the whole
// quit — open forever. Every client has already been sent its kill by the time
// the deadline can expire, so what is abandoned here is the join, not the
// teardown.
func (m *Manager) Stop(ctx context.Context) error {
	m.stopOnce.Do(func() {
		m.mu.Lock()
		m.stopped = true
		clients := make([]*managedClient, 0, len(m.clients))
		for _, mc := range m.clients {
			clients = append(clients, mc)
		}
		m.clients = map[string]*managedClient{}
		m.mu.Unlock()

		closed := make(chan struct{})
		go func() {
			defer close(closed)
			var wg sync.WaitGroup
			for _, mc := range clients {
				wg.Go(func() {
					_ = mc.client.Close(ctx)
					mc.cancel()
				})
			}
			wg.Wait()
		}()

		select {
		case <-closed:
		case <-ctx.Done():
			m.log.Warn().Int("clients", len(clients)).Msg("tmux clients did not close within the shutdown budget")
		}
		m.cancelAll()
	})
	return nil
}

// remove evicts slug only if it still holds the registration gen identifies. A
// client that exits after its slug was re-attached must not take the new client
// with it.
func (m *Manager) remove(slug string, gen uint64) {
	m.mu.Lock()
	mc, ok := m.clients[slug]
	if !ok || mc.gen != gen {
		m.mu.Unlock()
		return
	}
	delete(m.clients, slug)
	m.mu.Unlock()
	mc.cancel()
}

func tmuxVersion(ctx context.Context, binary string) (string, error) {
	out, err := exec.CommandContext(ctx, binary, "-V").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// versionAtLeast reads the first major.minor in a `tmux -V` string, which may
// be decorated ("tmux 3.2a", "tmux next-3.4").
func versionAtLeast(raw string, major, minor int) bool {
	i := strings.IndexFunc(raw, func(r rune) bool { return r >= '0' && r <= '9' })
	if i < 0 {
		return false
	}
	rest := raw[i:]
	j := 0
	for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
		j++
	}
	gotMajor, err := strconv.Atoi(rest[:j])
	if err != nil {
		return false
	}
	gotMinor := 0
	if j < len(rest) && rest[j] == '.' {
		rest = rest[j+1:]
		k := 0
		for k < len(rest) && rest[k] >= '0' && rest[k] <= '9' {
			k++
		}
		gotMinor, _ = strconv.Atoi(rest[:k])
	}
	return gotMajor > major || (gotMajor == major && gotMinor >= minor)
}
