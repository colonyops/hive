package main

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// occupant holds a loopback port and answers the health probe, standing in for
// whatever already owns the address. release frees the port.
func occupant(t *testing.T, isDevserver bool) (addr string, release func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	body := `{"devserver":false}`
	if isDevserver {
		body = `{"devserver":true}`
	}
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(body)) //nolint:errcheck // test server
		}),
		ReadHeaderTimeout: time.Second,
	}
	go func() { _ = server.Serve(listener) }()

	// sync.Once, not a captured bool: the takeover test releases from its own
	// goroutine and again from a defer, so the guard is read and written from
	// two goroutines at once.
	var once sync.Once
	return listener.Addr().String(), func() {
		once.Do(func() { _ = server.Close() })
	}
}

// freeAddr returns an address nothing is listening on.
func freeAddr(t *testing.T) string {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := probe.Addr().String()
	require.NoError(t, probe.Close())
	return addr
}

// An hour-long poll proves the free-port path never reaches the wait.
func TestAwaitListenerBindsAFreePortImmediately(t *testing.T) {
	addr := freeAddr(t)

	listener, err := awaitListener(t.Context(), addr, time.Hour, zerolog.Nop())
	require.NoError(t, err)
	defer listener.Close() //nolint:errcheck // test teardown

	assert.Equal(t, addr, listener.Addr().String())
}

// The point of standing by: a second launch is not a duplicate to be discarded,
// it is the proxy that takes over when the live one stops, so the worktrees
// pointed at the port keep working without anyone restarting anything.
func TestAwaitListenerTakesOverWhenThePortIsReleased(t *testing.T) {
	addr, release := occupant(t, true)
	defer release()

	go func() {
		time.Sleep(50 * time.Millisecond)
		release()
	}()

	listener, err := awaitListener(t.Context(), addr, 10*time.Millisecond, zerolog.Nop())
	require.NoError(t, err)
	defer listener.Close() //nolint:errcheck // test teardown

	assert.Equal(t, addr, listener.Addr().String())
}

// A stranger on the port is fatal rather than waited on: instances redirected at
// something that is not a proxy is the failure the singleton exists to prevent,
// and no amount of waiting fixes it.
func TestAwaitListenerRejectsAForeignOccupant(t *testing.T) {
	addr, release := occupant(t, false)
	defer release()

	_, err := awaitListener(t.Context(), addr, 10*time.Millisecond, zerolog.Nop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not devserver")
}

// Ctrl-C while standing by has to exit, not wait for a port that may never free.
func TestAwaitListenerStopsWhenInterrupted(t *testing.T) {
	addr, release := occupant(t, true)
	defer release()

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := awaitListener(ctx, addr, 10*time.Millisecond, zerolog.Nop())
	assert.ErrorIs(t, err, context.Canceled)
}
