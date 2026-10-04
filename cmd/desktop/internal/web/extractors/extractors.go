// Package extractors decodes and validates request input: Body decodes into a
// typed struct and runs its Validate method when it has one.
package extractors

import (
	"encoding/json"
	"net/http"

	"github.com/colonyops/hive/cmd/desktop/internal/web"
)

const maxBodyBytes = 1 << 20

// Validator is the optional contract a request struct implements to be
// validated after decoding.
type Validator interface {
	Validate() error
}

// Body decodes a JSON request body into T, capped at 1 MiB with unknown
// fields rejected, then validates it.
func Body[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var v T
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return v, &web.BadRequestError{Msg: "invalid request body", Err: err}
	}
	return v, validate(v)
}

func validate(v any) error {
	if val, ok := v.(Validator); ok {
		return val.Validate()
	}
	return nil
}
