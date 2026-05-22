package commands

import (
	"testing"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/stretchr/testify/assert"
)

func TestDetectTmuxSessionNames(t *testing.T) {
	sess := session.Session{
		Name: "Display Name",
		Slug: "display-name",
		Metadata: map[string]string{
			session.MetaTmuxSession: "tmux-display",
		},
	}

	got := detectTmuxSessionNames(sess)

	assert.True(t, got["tmux-display"])
	assert.True(t, got["display-name"])
	assert.True(t, got["Display Name"])
	assert.Len(t, got, 3)
}

func TestDetectTmuxSessionNames_Dedupes(t *testing.T) {
	sess := session.Session{
		Name: "same",
		Slug: "same",
		Metadata: map[string]string{
			session.MetaTmuxSession: "same",
		},
	}

	got := detectTmuxSessionNames(sess)

	assert.Equal(t, map[string]bool{"same": true}, got)
}
