package session

import (
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
)

// SessionTarget resolves the actual multiplexer session name for a Hive session.
func Target(sess session.Session) multiplexer.Target {
	name := sess.GetMeta(session.MetaTmuxSession)
	if name == "" {
		name = sess.Slug
	}
	return multiplexer.Target{Session: name}
}
