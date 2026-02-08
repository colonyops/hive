package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/hay-kot/hive/internal/core/git"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSessionLister struct {
	sessions []session.Session
	err      error
	git      git.Git
}

func (m *mockSessionLister) ListSessions(context.Context) ([]session.Session, error) {
	return m.sessions, m.err
}

func (m *mockSessionLister) Git() git.Git {
	return m.git
}

func TestLoadSessions_Success(t *testing.T) {
	sessions := []session.Session{
		{ID: "1", Name: "s1", Path: "/tmp/s1"},
		{ID: "2", Name: "s2", Path: "/tmp/s2"},
	}
	mock := &mockSessionLister{sessions: sessions}
	m := Model{service: mock}

	cmd := m.loadSessions()
	require.NotNil(t, cmd)

	msg := cmd()
	loaded, ok := msg.(sessionsLoadedMsg)
	require.True(t, ok)
	require.NoError(t, loaded.err)
	assert.Len(t, loaded.sessions, 2)
	assert.Equal(t, "s1", loaded.sessions[0].Name)
}

func TestLoadSessions_Error(t *testing.T) {
	mock := &mockSessionLister{err: errors.New("db error")}
	m := Model{service: mock}

	cmd := m.loadSessions()
	msg := cmd()
	loaded, ok := msg.(sessionsLoadedMsg)
	require.True(t, ok)
	assert.Error(t, loaded.err)
}
