package hive

import (
	"testing"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/stretchr/testify/assert"
)

func TestSessionTarget(t *testing.T) {
	sess := session.Session{Name: "Display Name", Slug: "display-name"}
	assert.Equal(t, "display-name", SessionTarget(sess).Session)

	sess.Metadata = map[string]string{session.MetaTmuxSession: "actual-session"}
	assert.Equal(t, "actual-session", SessionTarget(sess).Session)

	sess.Slug = ""
	sess.Metadata = nil
	assert.Empty(t, SessionTarget(sess).Session, "display name is never a tmux target fallback")
}
