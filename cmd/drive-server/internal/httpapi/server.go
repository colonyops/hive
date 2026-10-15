// Package httpapi is the drive server's HTTP adapter.
package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/web/mid"
	"github.com/colonyops/hive/pkg/buildinfo"
)

const shutdownGrace = 10 * time.Second

// Services are the core services the handlers call.
type Services struct {
	Tokens    *core.Tokens
	Documents *core.Documents
	Shares    *core.Shares
}

// New returns the server's handler. /healthz is quiet in the request log
// unless it fails, so a probe loop does not flood it.
func New(log zerolog.Logger, info buildinfo.Info, svc Services) http.Handler {
	chain := errchain.New(mid.Errors(log, mapError))
	mux := http.NewServeMux()
	registerRoutes(mux, chain, info, svc)
	return mid.Logger(log, "/healthz")(mux)
}

// Serve runs h on ln until ctx is done, then gives in-flight requests
// shutdownGrace to finish.
func Serve(ctx context.Context, ln net.Listener, h http.Handler, log zerolog.Logger) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
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
