package mcpsrv

import (
	"errors"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// toolError maps a core error's Kind onto what an agent reads, exactly once
// for this adapter.
//
// The SDK packs an error returned from a tool handler into the tool result
// with IsError set, rather than failing the JSON-RPC call — which is the
// behaviour we want for every Kind: an agent that named a profile which does
// not exist should read "not_found" and correct itself, not lose its session
// to a protocol error.
//
// The Kind leads the message so it stays machine-readable without the agent
// parsing prose, the same contract the REST surface had when it put
// {kind, message} on the wire.
//
// The wrapped cause crosses too, which is where this deliberately diverges
// from app.Error's own MarshalJSON: the frontend shows Msg to a person and
// keeps the cause for the log, but Msg is written as a context prefix
// ("clearing image for profile \"x\""), so dropping the cause hands an agent a
// dangling phrase with no reason in it. The consumer here is a model debugging
// its own call on the user's machine — the detail is the answer, not a leak.
func (ctrl *Controller) toolError(err error) error { return toolError(ctrl.log, err) }

func toolError(log zerolog.Logger, err error) error {
	if err == nil {
		return nil
	}
	if appErr, ok := errors.AsType[*app.Error](err); ok {
		if appErr.Err != nil {
			log.Debug().Err(err).Str("kind", string(appErr.Kind)).Msg("mcp tool refused a call")
			// Rendered, not wrapped: what crosses is text for a model to read,
			// and the chain itself has no meaning on the far side of JSON-RPC.
			return fmt.Errorf("%s: %s: %s", appErr.Kind, appErr.Msg, appErr.Err.Error())
		}
		return fmt.Errorf("%s: %s", appErr.Kind, appErr.Msg)
	}
	// Unclassified means internal by definition — nobody decided otherwise.
	// Its text is for the log, not for the model.
	log.Error().Err(err).Msg("mcp tool failed")
	return fmt.Errorf("%s: the app could not complete the request", app.KindInternal)
}
