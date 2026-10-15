// Package app wires the drive server: the HTTP server and the plugs manager
// that runs it.
package app

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/hay-kot/appkit/plugs"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/httpapi"
	"github.com/colonyops/hive/pkg/buildinfo"
	"github.com/colonyops/hive/pkg/logutils"
)

type Options struct {
	Info buildinfo.Info
}

// Run serves until ctx is done or a plug fails.
func Run(ctx context.Context, cfg config.Config, log zerolog.Logger, opts Options) error {
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Listen, err)
	}
	defer func() { _ = ln.Close() }()

	httpLog := logutils.Component(log, "httpapi")
	handler := httpapi.New(httpLog, opts.Info)

	log.Info().Str("addr", ln.Addr().String()).Str("base_url", cfg.BaseURL).Str("version", opts.Info.Version).Msg("drive server listening")

	mgr := plugs.New(
		plugs.WithTimeout(10*time.Second),
		plugs.WithLogger(plugsLogger{logutils.Component(log, "plugs")}),
		plugs.WithMaxAttempts(3),
	)
	mgr.Add(plugs.PluginFunc("http", func(ctx context.Context) error {
		return httpapi.Serve(ctx, ln, handler, httpLog)
	}))
	return mgr.Start(ctx)
}

type plugsLogger struct{ log zerolog.Logger }

func (l plugsLogger) Debug(msg string, kv ...any) { l.log.Debug().Fields(kv).Msg(msg) }
func (l plugsLogger) Info(msg string, kv ...any)  { l.log.Info().Fields(kv).Msg(msg) }
func (l plugsLogger) Warn(msg string, kv ...any)  { l.log.Warn().Fields(kv).Msg(msg) }
func (l plugsLogger) Error(msg string, kv ...any) { l.log.Error().Fields(kv).Msg(msg) }
