package httpapi

import (
	"errors"
	"net/http"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/web"
)

// mapError renders core errors and oversized bodies. Malformed input
// (web.BadRequestError, 400) is handled before mappers run, so it never
// reaches here.
func mapError(err error) (int, any, bool) {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return http.StatusRequestEntityTooLarge, web.ErrorBody{Kind: string(core.KindTooLarge), Message: "request body too large"}, true
	}
	coreErr, ok := errors.AsType[*core.Error](err)
	if !ok {
		return 0, nil, false
	}
	status, ok := statusForKind(coreErr.Kind)
	if !ok {
		return 0, nil, false
	}
	return status, web.ErrorBody{Kind: string(coreErr.Kind), Message: coreErr.Message}, true
}

func statusForKind(k core.Kind) (int, bool) {
	switch k {
	case core.KindInvalid:
		return http.StatusUnprocessableEntity, true
	case core.KindForbidden:
		return http.StatusForbidden, true
	case core.KindNotFound:
		return http.StatusNotFound, true
	case core.KindConflict:
		return http.StatusConflict, true
	case core.KindUnauthenticated:
		return http.StatusUnauthorized, true
	case core.KindTooLarge:
		return http.StatusRequestEntityTooLarge, true
	case core.KindRateLimited:
		return http.StatusTooManyRequests, true
	default:
		return 0, false
	}
}
