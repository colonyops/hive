package httpapi

import (
	"errors"
	"net/http"
	"runtime/debug"

	"github.com/hay-kot/httpkit/server"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/web"
)

// recoverPanics answers a panicking handler with the JSON 500 every other
// failure gets, instead of net/http's dropped connection. http.ErrAbortHandler
// is rethrown: it is how a handler asks net/http to abort the response.
func recoverPanics(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v)
				}
				log.Error().
					Interface("panic", v).
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Str("stack", string(debug.Stack())).
					Msg("handler panicked")
				_ = server.JSON(w, http.StatusInternalServerError, web.ErrorBody{Kind: "internal", Message: "internal error"})
			}()
			next.ServeHTTP(w, r)
		})
	}
}
