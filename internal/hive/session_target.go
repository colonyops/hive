package hive

import (
	"github.com/colonyops/hive/internal/core/multiplexer"
	"github.com/colonyops/hive/internal/core/session"
)

// SessionTarget resolves the actual multiplexer session name for a Hive session.
func SessionTarget(sess session.Session) multiplexer.Target {
	name := sess.GetMeta(session.MetaTmuxSession)
	if name == "" {
		name = sess.Slug
	}
	return multiplexer.Target{Session: name}
}
