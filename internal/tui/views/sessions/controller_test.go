package sessions

import (
	"testing"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSessions() []session.Session {
	return []session.Session{
		{ID: "aaa", Name: "alpha", Remote: "https://github.com/org/repo-a"},
		{ID: "bbb", Name: "beta", Remote: "https://github.com/org/repo-a"},
		{ID: "ccc", Name: "gamma", Remote: "https://github.com/org/repo-b"},
	}
}

func TestController_SetAndGetSessions(t *testing.T) {
	ctrl := NewController()

	assert.Empty(t, ctrl.AllSessions())

	sessions := testSessions()
	ctrl.SetSessions(sessions)
	assert.Equal(t, sessions, ctrl.AllSessions())
}

func TestController_FindByID(t *testing.T) {
	ctrl := NewController()
	ctrl.SetSessions(testSessions())

	tests := []struct {
		name   string
		id     string
		wantOK bool
	}{
		{name: "existing session", id: "bbb", wantOK: true},
		{name: "first session", id: "aaa", wantOK: true},
		{name: "last session", id: "ccc", wantOK: true},
		{name: "missing session", id: "zzz", wantOK: false},
		{name: "empty id", id: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ctrl.FindByID(tt.id)
			if tt.wantOK {
				require.NotNil(t, result)
				assert.Equal(t, tt.id, result.ID)
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

func TestController_StatusFilter(t *testing.T) {
	ctrl := NewController()

	assert.Equal(t, terminal.Status(""), ctrl.StatusFilter())

	ctrl.SetStatusFilter(terminal.StatusActive)
	assert.Equal(t, terminal.StatusActive, ctrl.StatusFilter())

	ctrl.SetStatusFilter("")
	assert.Equal(t, terminal.Status(""), ctrl.StatusFilter())
}

func TestController_LocalRemote(t *testing.T) {
	ctrl := NewController()

	assert.Empty(t, ctrl.LocalRemote())

	ctrl.SetLocalRemote("https://github.com/org/repo")
	assert.Equal(t, "https://github.com/org/repo", ctrl.LocalRemote())
}

func TestController_SetSessions_Replaces(t *testing.T) {
	ctrl := NewController()

	first := []session.Session{{ID: "aaa", Name: "first"}}
	second := []session.Session{{ID: "bbb", Name: "second"}}

	ctrl.SetSessions(first)
	assert.Len(t, ctrl.AllSessions(), 1)
	assert.Equal(t, "aaa", ctrl.AllSessions()[0].ID)

	ctrl.SetSessions(second)
	assert.Len(t, ctrl.AllSessions(), 1)
	assert.Equal(t, "bbb", ctrl.AllSessions()[0].ID)

	// Old session no longer findable
	assert.Nil(t, ctrl.FindByID("aaa"))
	assert.NotNil(t, ctrl.FindByID("bbb"))
}
