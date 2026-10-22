package httpapi

import (
	"errors"
	"net/http"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/web"
)

// mapError renders core errors. Malformed input (400) and oversized bodies
// (413) are handled by mid.Errors before mappers run.
func mapError(err error) (int, any, bool) {
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
