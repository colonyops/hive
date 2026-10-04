package mid

import (
	"net/http"
	"slices"
	"time"

	"github.com/rs/zerolog"
)

type spy struct {
	http.ResponseWriter
	status int
}

func (s *spy) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func (s *spy) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *spy) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Logger logs one line per completed request. Requests to a skip path only
// log on a non-2xx status, so a poll loop does not flood the log.
func Logger(l zerolog.Logger, skip ...string) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s := &spy{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			h.ServeHTTP(s, r)

			if s.status/100 == 2 && slices.Contains(skip, r.URL.Path) {
				return
			}
			l.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", s.status).
				Dur("duration", time.Since(start)).
				Msg("request complete")
		})
	}
}
