package profiler

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/rs/zerolog/log"
)

type Server struct {
	httpServer *http.Server
	listener   net.Listener
	port       int
}

func New(port int) *Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return &Server{
		httpServer: &http.Server{
			Handler: mux,
		},
		port: port,
	}
}

func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	s.listener = listener

	actualPort := listener.Addr().(*net.TCPAddr).Port
	log.Info().Int("port", actualPort).Msg("starting profiler server")

	errChan := make(chan error, 1)
	go func() {
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case err := <-errChan:
		return fmt.Errorf("profiler server failed to start: %w", err)
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Info().Msg("shutting down profiler server")
	return s.httpServer.Shutdown(ctx)
}
