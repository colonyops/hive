package daemonclient

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"

	"github.com/colonyops/hive/internal/hive/daemon/daemonpath"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	"github.com/colonyops/hive/pkg/flock"
)

// restartWait bounds how long a restart waits for the old daemon to let go
// of its socket and lock.
const restartWait = 5 * time.Second

const restartPollInterval = 25 * time.Millisecond

// skipGate marks a call the gate must pass without a check, such as the Stop
// the gate sends itself.
type skipGate struct{}

// protocolGate runs Hello before the first other call on each new connection
// and refuses every call while the daemon's protocol is incompatible. A new
// connection checks again, since the daemon may have been upgraded.
//
// With replaceOlder it also restarts a local daemon that is an older release
// than the client. A daemon never exits on its own, so it outlives app and
// CLI updates and keeps serving the code it started with.
type protocolGate struct {
	client        *Client
	clientVersion string
	replaceOlder  bool
	dataDir       string

	// slow serializes the checks that talk to the daemon, so concurrent calls
	// on a new connection send one Hello and make one restart decision. It is
	// not mu, which a dial takes through reset.
	slow      sync.Mutex
	restarted bool
	warned    bool

	mu      sync.Mutex
	checked bool
	err     error
}

func (g *protocolGate) reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checked = false
	g.err = nil
}

func (g *protocolGate) verdict() (checked bool, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.checked, g.err
}

func (g *protocolGate) check(ctx context.Context) error {
	if checked, err := g.verdict(); checked {
		return err
	}
	g.slow.Lock()
	defer g.slow.Unlock()
	if checked, err := g.verdict(); checked {
		return err
	}
	// Not under mu: Hello may dial, and a dial calls reset. One attempt: the read
	// behind this check retries by itself, and two retries multiply the wait.
	info, err := g.client.helloOnce(ctx)
	if err != nil {
		// Not recorded: a transport failure says nothing about the protocol.
		return err
	}
	if g.replaceOlder {
		if info, err = g.replaceIfOlder(ctx, info); err != nil {
			return err
		}
	}
	err = wire.CheckProtocol(info.ProtocolVersion, info.Version, g.clientVersion)
	g.mu.Lock()
	g.checked, g.err = true, err
	g.mu.Unlock()
	return err
}

// replaceIfOlder stops a daemon that is an older release than this client and
// lets the autostart path spawn this client's binary in its place. It restarts
// at most once per client: a daemon that is still older afterwards runs from
// somewhere this client cannot change, such as a service unit that names an
// old binary. Agents survive because tmux servers outlive the daemon.
func (g *protocolGate) replaceIfOlder(ctx context.Context, info Info) (Info, error) {
	if !olderRelease(info.Version, g.clientVersion) {
		return info, nil
	}
	if g.restarted {
		g.warnStillOlder(info)
		return info, nil
	}
	g.restarted = true
	logger := g.client.logger
	logger.Info().Str("daemon", info.Version).Str("client", g.clientVersion).Msg("restarting the daemon: it is older than this client")

	// Another client may have stopped it first, which is the outcome wanted.
	if err := g.client.Stop(context.WithValue(ctx, skipGate{}, true)); err != nil && !transportFailure(err) {
		logger.Warn().Err(err).Msg("could not stop the older daemon; continuing with it")
		return info, nil
	}
	g.client.httpClient.CloseIdleConnections()
	if !g.waitForExit(ctx) {
		logger.Warn().Msg("the older daemon still answers after the restart wait; continuing")
	}
	// Hello dials, and a dial with no daemon spawns this client's binary.
	fresh, err := g.client.Hello(ctx)
	if err != nil {
		return Info{}, err
	}
	if olderRelease(fresh.Version, g.clientVersion) {
		g.warnStillOlder(fresh)
	}
	return fresh, nil
}

func (g *protocolGate) warnStillOlder(info Info) {
	if g.warned {
		return
	}
	g.warned = true
	g.client.logger.Warn().
		Str("daemon", info.Version).
		Str("client", g.clientVersion).
		Msg("the daemon is older than this client and a restart did not replace it; run `hive daemon install` to rewrite the service unit")
}

// waitForExit waits until nothing answers on the socket and, when the data
// directory is known, the daemon lock is free. A spawn while the old daemon
// still holds the lock would exit at once and leave no daemon.
func (g *protocolGate) waitForExit(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, restartWait)
	defer cancel()
	ticker := time.NewTicker(restartPollInterval)
	defer ticker.Stop()
	for {
		conn, err := g.client.dialer(ctx)
		switch {
		case err == nil:
			_ = conn.Close()
		case !noDaemon(err):
			return true
		case g.lockFree(ctx):
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

func (g *protocolGate) lockFree(ctx context.Context) bool {
	if g.dataDir == "" {
		return true
	}
	unlock, err := flock.Lock(ctx, daemonpath.LockPath(g.dataDir), false)
	if errors.Is(err, flock.ErrLocked) {
		return false
	}
	if err == nil {
		_ = unlock()
	}
	return true
}

func isHello(procedure string) bool {
	return strings.HasSuffix(procedure, "/Hello")
}

func (g *protocolGate) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if !isHello(req.Spec().Procedure) && ctx.Value(skipGate{}) == nil {
			if err := g.check(ctx); err != nil {
				return nil, err
			}
		}
		return next(ctx, req)
	}
}

func (g *protocolGate) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		if err := g.check(ctx); err != nil {
			return &failedStream{StreamingClientConn: next(ctx, spec), err: err}
		}
		return next(ctx, spec)
	}
}

func (g *protocolGate) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

// failedStream reports err from every send and receive.
type failedStream struct {
	connect.StreamingClientConn
	err error
}

func (s *failedStream) Send(any) error    { return s.err }
func (s *failedStream) Receive(any) error { return s.err }
