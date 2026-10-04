// Package mid holds the errchain error middleware shared by the JSON APIs.
package mid

import (
	"errors"
	"net/http"

	"github.com/hay-kot/criterio"
	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/web"
)

// Mapper translates one API's own error types to a response; the first mapper
// to report ok wins.
type Mapper func(err error) (status int, body any, ok bool)

// Errors turns handler errors into responses: unreadable body 400, validation
// failure 422, then mappers, then an opaque logged 500.
func Errors(log zerolog.Logger, mappers ...Mapper) errchain.ErrorHandler {
	return func(h errchain.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := h.ServeHTTP(w, r)
			if err == nil {
				return
			}

			var badReq *web.BadRequestError
			var fieldErrs criterio.FieldErrors
			switch {
			case errors.As(err, &badReq):
				_ = server.JSON(w, http.StatusBadRequest, web.ErrorBody{
					Kind: "invalid", Message: badReq.Error(),
				})
			case errors.As(err, &fieldErrs):
				fields := make(map[string]string, len(fieldErrs))
				for _, fe := range fieldErrs {
					fields[fe.Field] = fe.Err.Error()
				}
				_ = server.JSON(w, http.StatusUnprocessableEntity, web.ErrorBody{
					Kind: "invalid", Message: "invalid request", Fields: fields,
				})
			default:
				for _, m := range mappers {
					if status, body, ok := m(err); ok {
						if status >= http.StatusInternalServerError {
							log.Error().Err(err).Str("path", r.URL.Path).Msg("internal error response")
						}
						_ = server.JSON(w, status, body)
						return
					}
				}
				log.Error().Err(err).Str("path", r.URL.Path).Msg("unhandled error resulted in 500 response")
				_ = server.JSON(w, http.StatusInternalServerError, web.ErrorBody{
					Kind: "internal", Message: "internal error",
				})
			}
		})
	}
}
