package remoteconn

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/colonyops/hive/pkg/atomicfile"
	"github.com/colonyops/hive/pkg/logutils"
	"github.com/rs/zerolog"
)

type Credentials struct {
	URL         string `json:"url"`
	Token       string `json:"token"`
	Certificate string `json:"certificate"`
}

type Server struct {
	Address        string
	Directory      string
	ConnectionFile string
	Token          string
	Handler        http.Handler
	Logger         zerolog.Logger
	server         *http.Server
	cancel         context.CancelFunc
	once           sync.Once
}

func (s *Server) Start(ctx context.Context) error {
	s.Logger = logutils.Component(s.Logger, "remote-tls")
	pair, cert, err := certificate(s.Directory)
	if err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(s.Address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("remote TLS listener requires a loopback address")
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", s.Address)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.server = &http.Server{Handler: s.Handler, ReadHeaderTimeout: 5 * time.Second, BaseContext: func(net.Listener) context.Context { return runCtx }}
	data, err := json.Marshal(Credentials{URL: "https://" + ln.Addr().String(), Token: s.Token, Certificate: cert})
	if err == nil {
		err = atomicfile.Write(s.ConnectionFile, append(data, '\n'), 0o600)
	}
	if err != nil {
		cancel()
		_ = ln.Close()
		return fmt.Errorf("export remote connection: %w", err)
	}
	go func() {
		err := s.server.Serve(tls.NewListener(ln, &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}}))
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.Logger.Error().Err(err).Msg("remote TLS listener stopped")
		}
	}()
	return nil
}

func (s *Server) Close() error {
	var err error
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.server != nil {
			err = s.server.Close()
		}
	})
	return err
}
