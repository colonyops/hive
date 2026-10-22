// Package httpapi is the drive server's HTTP adapter.
package httpapi

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/web/mid"
	"github.com/colonyops/hive/pkg/buildinfo"
)

const (
	shutdownGrace = 10 * time.Second

	readTimeoutFloor = 30 * time.Second
	// minUploadRate is the slowest client, in bytes per second, that can
	// still upload a document of the largest accepted size.
	minUploadRate = 64 << 10
)

type Services struct {
	Tokens    *core.Tokens
	Sessions  *core.Sessions
	SignIn    *core.SignIn
	Documents *core.Documents
	Shares    *core.Shares

	// Web is the built web UI. When nil, no UI route is registered.
	Web fs.FS

	// BaseURL decides the session cookie's name and Secure flag.
	BaseURL string
	// TrustedProxies are the proxies whose X-Forwarded-For names the client.
	TrustedProxies []netip.Prefix
}

// New returns the server's handler. /healthz is quiet in the request log
// unless it fails, so a probe loop does not flood it.
func New(log zerolog.Logger, info buildinfo.Info, svc Services) http.Handler {
	chain := errchain.New(mid.Errors(log, mapError))
	mux := http.NewServeMux()
	registerRoutes(mux, chain, info, svc)
	// Cookie requests are cross-origin-checked here; bearer calls send no
	// Origin and pass.
	return mid.Logger(log, "/healthz")(recoverPanics(log)(http.NewCrossOriginProtection().Handler(mux)))
}

// Serve runs h on ln until ctx is done, then gives in-flight requests
// shutdownGrace to finish. maxDocumentSize sizes the read timeout so the
// largest accepted upload fits.
func Serve(ctx context.Context, ln net.Listener, h http.Handler, log zerolog.Logger, maxDocumentSize int64) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       readTimeout(maxDocumentSize),
		IdleTimeout:       60 * time.Second,
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	log.Info().Msg("shutting down http server")
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// readTimeout is zero, no limit, when documents are uncapped: no fixed timeout
// would fit an unbounded upload.
func readTimeout(maxDocumentSize int64) time.Duration {
	if maxDocumentSize <= 0 {
		return 0
	}
	return readTimeoutFloor + time.Duration(maxDocumentSize/minUploadRate)*time.Second
}
