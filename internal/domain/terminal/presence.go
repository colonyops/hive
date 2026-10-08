package terminal

import "context"

// SessionPresenceReader confirms session existence independently of agent detection.
// An unknown result must not be treated as termination.
type SessionPresenceReader interface {
	SessionPresence(context.Context, string, map[string]string) (present, known bool, err error)
}
