package wailsui

import (
	"encoding/json"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// wireError is the shape a failed binding call carries to the frontend as the
// thrown exception's `cause`. It is deliberately not app.Error: the cause
// chain is for the log, and a message the core wrote for a developer should
// not be the only thing distinguishing two failures a UI must handle
// differently. Kind is what the frontend switches on.
type wireError struct {
	Kind    app.Kind `json:"kind"`
	Message string   `json:"message"`
}

// MarshalError maps a core error to the JSON the frontend receives. It is the
// single place Kind is translated for this adapter, wired once at
// application.Options.MarshalError.
//
// An unclassified error is reported as internal, which is what KindOf already
// decides: nobody said otherwise, so the caller can do nothing about it.
func MarshalError(err error) []byte {
	if err == nil {
		return nil
	}
	encoded, marshalErr := json.Marshal(wireError{Kind: app.KindOf(err), Message: err.Error()})
	if marshalErr != nil {
		// A failure to encode a failure must still say something the frontend
		// can parse, or the call surfaces as an unexplained rejection.
		return []byte(`{"kind":"internal","message":"the error could not be encoded"}`)
	}
	return encoded
}
