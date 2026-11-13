package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/platform/herdrcore"
)

// herdrSessionLabel ties a herdr-core terminal to the hive session it was
// opened for.
const herdrSessionLabel = "hive.session"

// HerdrTerminalsService runs a session's native terminals in herdr-core
// (`hc`), which keeps each program alive across app restarts and relays its
// bytes to whatever draws it. One connection serves every call; it is dialed
// on first use and again after it drops.
type HerdrTerminalsService struct {
	environ   func(context.Context) []string
	lookPath  func(context.Context, string) (string, error)
	directory terminalDirectory
	logger    zerolog.Logger

	mu     sync.Mutex
	client *herdrcore.Client
}

// HerdrTerminalsDeps are HerdrTerminalsService's collaborators.
type HerdrTerminalsDeps struct {
	// Environ is the login environment the core starts with, when hive has
	// to start one.
	Environ   func(context.Context) []string
	LookPath  func(context.Context, string) (string, error)
	Directory terminalDirectory
	Logger    zerolog.Logger
}

func newHerdrTerminalsService(d HerdrTerminalsDeps) *HerdrTerminalsService {
	return &HerdrTerminalsService{environ: withCoreSelection(d.Environ, os.Getenv), lookPath: d.LookPath, directory: d.Directory, logger: d.Logger}
}

// withCoreSelection lets HC_SESSION and HC_SOCKET set on the app process pick
// the core, over the login environment's: a dev instance runs its own core
// rather than the user's.
func withCoreSelection(environ func(context.Context) []string, getenv func(string) string) func(context.Context) []string {
	return func(ctx context.Context) []string {
		env := environ(ctx)
		for _, name := range []string{"HC_SESSION", "HC_SOCKET"} {
			if v := getenv(name); v != "" {
				env = append(env, name+"="+v)
			}
		}
		return env
	}
}

// Available reports whether herdr-core is installed.
func (s *HerdrTerminalsService) Available(ctx context.Context) bool {
	_, err := s.lookPath(ctx, "hc")
	return err == nil
}

// Socket is where the core this service uses listens.
func (s *HerdrTerminalsService) Socket(ctx context.Context) (string, error) {
	return herdrcore.SocketPath(s.getenv(ctx))
}

// Start opens the machine's shell in a new terminal in slug's checkout. The
// terminal is kept: a lost program comes back as a fresh shell.
func (s *HerdrTerminalsService) Start(ctx context.Context, slug string) (herdrcore.Terminal, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return herdrcore.Terminal{}, Errorf(KindInvalid, "session slug is required")
	}
	if _, _, remote := splitTarget(slug); remote {
		return herdrcore.Terminal{}, Errorf(KindUnavailable, "native terminals run on this machine; session %q is on a server", slug)
	}
	dir, err := s.directory.SessionDirectory(ctx, slug)
	if err != nil {
		return herdrcore.Terminal{}, err
	}
	c, err := s.connect(ctx)
	if err != nil {
		return herdrcore.Terminal{}, err
	}
	t, err := c.Run(ctx, herdrcore.RunParams{
		Cwd:      dir,
		Keep:     true,
		NoParent: true,
		Labels:   map[string]string{herdrSessionLabel: slug},
	})
	return t, herdrError(err, "start a terminal for %q", slug)
}

// Get returns one terminal's record.
func (s *HerdrTerminalsService) Get(ctx context.Context, address string) (herdrcore.Terminal, error) {
	c, err := s.connect(ctx)
	if err != nil {
		return herdrcore.Terminal{}, err
	}
	t, err := c.Get(ctx, address)
	return t, herdrError(err, "read terminal %s", address)
}

// Kill ends a terminal's program. A terminal that has already gone is not
// an error.
func (s *HerdrTerminalsService) Kill(ctx context.Context, address string) error {
	c, err := s.connect(ctx)
	if err != nil {
		return err
	}
	err = c.Kill(ctx, address)
	if rpcErr, ok := errors.AsType[*herdrcore.RPCError](err); ok && (rpcErr.Kind() == "not_found" || rpcErr.Kind() == "finished") {
		return nil
	}
	return herdrError(err, "end terminal %s", address)
}

// Close drops the connection. Terminals keep running in the core.
func (s *HerdrTerminalsService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == nil {
		return nil
	}
	err := s.client.Close()
	s.client = nil
	return err
}

func (s *HerdrTerminalsService) connect(ctx context.Context) (*herdrcore.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		select {
		case <-s.client.Done():
			s.client = nil
		default:
			return s.client, nil
		}
	}
	socket, err := s.Socket(ctx)
	if err != nil {
		return nil, Wrap(err, KindInvalid, "find herdr-core")
	}
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	c, err := herdrcore.Dial(dialCtx, socket)
	cancel()
	if err != nil {
		if startErr := s.startCore(ctx); startErr != nil {
			return nil, startErr
		}
		dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if c, err = herdrcore.Dial(dialCtx, socket); err != nil {
			return nil, Wrap(err, KindUnavailable, "reach herdr-core at %s", socket)
		}
	}
	s.client = c
	return c, nil
}

// startCore starts a core in the login environment, so its terminals get
// the user's PATH and variables rather than the app bundle's.
func (s *HerdrTerminalsService) startCore(ctx context.Context) error {
	hc, err := s.lookPath(ctx, "hc")
	if err != nil {
		return Wrap(err, KindUnavailable, "herdr-core (hc) is not installed")
	}
	cmd := exec.CommandContext(ctx, hc, "server", "start")
	cmd.Env = s.environ(ctx)
	if out, err := cmd.CombinedOutput(); err != nil {
		return Wrap(err, KindUnavailable, "start herdr-core: %s", strings.TrimSpace(string(out)))
	}
	s.logger.Info().Str("hc", hc).Msg("started herdr-core")
	return nil
}

func (s *HerdrTerminalsService) getenv(ctx context.Context) func(string) string {
	env := s.environ(ctx)
	return func(name string) string {
		for _, kv := range slices.Backward(env) {
			if v, ok := strings.CutPrefix(kv, name+"="); ok {
				return v
			}
		}
		return ""
	}
}

func herdrError(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	if rpcErr, ok := errors.AsType[*herdrcore.RPCError](err); ok {
		switch rpcErr.Kind() {
		case "not_found", "machine_unknown":
			return Wrap(err, KindNotFound, format, args...)
		case "invalid_params", "usage":
			return Wrap(err, KindInvalid, format, args...)
		}
	}
	if errors.Is(err, herdrcore.ErrClosed) {
		return Wrap(err, KindUnavailable, format, args...)
	}
	return Wrap(err, KindInternal, format, args...)
}
