package mcpsrv

import (
	"errors"
	"fmt"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

// toolError renders err for the agent with its kind first, so a caller can
// branch on "not_found" without parsing prose. The SDK turns a returned error
// into a tool result with IsError set rather than failing the call.
func (c *Controller) toolError(err error) error {
	if err == nil {
		return nil
	}
	if e, ok := errors.AsType[*relay.Error](err); ok {
		if e.Message == "" && e.Err == nil {
			return errors.New(string(e.Kind))
		}
		return fmt.Errorf("%s: %s", e.Kind, e.Error())
	}
	c.log.Error().Err(err).Msg("mcp tool failed")
	return errors.New("internal: the relay could not complete the request")
}

func invalid(err error) error {
	if err == nil {
		return nil
	}
	return &relay.Error{Kind: relay.KindInvalid, Message: err.Error()}
}
