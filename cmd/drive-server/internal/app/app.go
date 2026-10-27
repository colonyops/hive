// Package app wires the drive server: the core services over control.db, the
// HTTP server, and the plugs manager that runs it.
package app

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/hay-kot/appkit/plugs"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/drive-server/frontend"
	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/guard"
	"github.com/colonyops/hive/cmd/drive-server/internal/httpapi"
	"github.com/colonyops/hive/cmd/drive-server/internal/mail"
	"github.com/colonyops/hive/pkg/buildinfo"
	"github.com/colonyops/hive/pkg/logutils"
)

const (
	sweepEvery = time.Minute
	mailEvery  = 5 * time.Second
)

type Options struct {
	Info buildinfo.Info
}

// Run serves until ctx is done or a plug fails.
func Run(ctx context.Context, cfg config.Config, log zerolog.Logger, opts Options) error {
	sender, err := newSender(cfg, os.Stdout)
	if err != nil {
		return err
	}
	trusted, err := guard.ParseProxies(cfg.TrustedProxies)
	if err != nil {
		return err
	}
	svc, err := OpenServices(ctx, log, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := svc.Close(); err != nil {
			log.Warn().Err(err).Msg("close control store")
		}
	}()

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Listen, err)
	}
	defer func() { _ = ln.Close() }()

	httpLog := logutils.Component(log, "httpapi")
	httpSvc := svc.HTTP(cfg.BaseURL, trusted)
	httpSvc.Web = frontend.Dist()
	handler := httpapi.New(httpLog, opts.Info, httpSvc)

	log.Info().Str("addr", ln.Addr().String()).Str("base_url", cfg.BaseURL).Str("version", opts.Info.Version).Msg("drive server listening")

	mgr := plugs.New(
		plugs.WithTimeout(10*time.Second),
		plugs.WithLogger(plugsLogger{logutils.Component(log, "plugs")}),
		plugs.WithMaxAttempts(3),
	)
	mgr.Add(plugs.PluginFunc("http", func(ctx context.Context) error {
		return httpapi.Serve(ctx, ln, handler, httpLog, cfg.MaxDocumentSize.Bytes)
	}))
	mailer := mail.NewMailer(log, svc.Outbox, sender, nil)
	mgr.Add(plugs.PluginFunc("mailer", func(ctx context.Context) error {
		return mailer.Run(ctx, mailEvery)
	}))
	mgr.Add(plugs.PluginFunc("sweeper", func(ctx context.Context) error {
		return svc.Sweeper.Run(ctx, sweepEvery)
	}))
	return mgr.Start(ctx)
}

type plugsLogger struct{ log zerolog.Logger }

func (l plugsLogger) Debug(msg string, kv ...any) { l.log.Debug().Fields(kv).Msg(msg) }
func (l plugsLogger) Info(msg string, kv ...any)  { l.log.Info().Fields(kv).Msg(msg) }
func (l plugsLogger) Warn(msg string, kv ...any)  { l.log.Warn().Fields(kv).Msg(msg) }
func (l plugsLogger) Error(msg string, kv ...any) { l.log.Error().Fields(kv).Msg(msg) }
