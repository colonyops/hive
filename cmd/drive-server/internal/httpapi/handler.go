package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/domain/drive"
)

// authed adapts fn to a route behind Authenticate: it hands fn the caller's
// principal and writes fn's result as JSON with status.
func authed[T any](status int, fn func(w http.ResponseWriter, r *http.Request, p core.Principal) (T, error)) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p, err := principal(r)
		if err != nil {
			return err
		}
		out, err := fn(w, r, p)
		if err != nil {
			return err
		}
		return server.JSON(w, status, out)
	}
}

// authedNoContent is authed for a route that answers 204.
func authedNoContent(fn func(r *http.Request, p core.Principal) error) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p, err := principal(r)
		if err != nil {
			return err
		}
		if err := fn(r, p); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

// spaceOf names the space a route addresses. A name that is not a valid
// space is a 404, not a 422: no such space can exist.
func spaceOf(r *http.Request, space spaceFunc) (drive.Space, error) {
	s, err := space(r)
	if err != nil {
		return drive.Space{}, core.NewError(core.KindNotFound, "space not found")
	}
	return s, nil
}
