package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/hive/internal/tui/views/sessions"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/store"
	"github.com/colonyops/hive/internal/store/db"
)

func newEditSessionTestModel(t *testing.T) Model {
	t.Helper()
	m := newKeybindingPrecedenceModel(t, nil)
	m = switchTestView(t, m, ViewSessions)
	item := sessions.TreeItem{Session: session.Session{
		ID: "s1", Name: "alpha", State: session.StateActive, Tags: []string{"auto", "backend"},
	}}
	listModel := getUnexportedField[list.Model](t, m.sessionsView, "list")
	listModel.SetItems([]list.Item{item})
	setUnexportedField(t, m.sessionsView, "list", listModel)
	return m
}

// pressKey sends key and delivers the action request the sessions view
// returns for it.
func pressKey(t *testing.T, m Model, key tea.KeyPressMsg) Model {
	t.Helper()
	model, cmd := m.Update(key)
	m = model.(Model)
	require.NotNil(t, cmd)
	model, _ = m.Update(cmd())
	return model.(Model)
}

func editSessionKey(t *testing.T, m Model, keyStr string, code rune) Model {
	t.Helper()
	model, _ := m.handleEditSessionKey(tea.KeyPressMsg(tea.Key{Code: code}), keyStr)
	return model.(Model)
}

func TestEditSessionOpensFocusedOnTheFieldItsKeyNames(t *testing.T) {
	tests := []struct {
		name  string
		key   tea.KeyPressMsg
		focus editSessionField
	}{
		{"rename key focuses the name", tea.KeyPressMsg{Code: 'R', Text: "R"}, editFieldName},
		{"tags key focuses the tags", tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}, editFieldTags},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := pressKey(t, newEditSessionTestModel(t), tt.key)

			assert.Equal(t, stateEditingSession, m.state)
			assert.Equal(t, tt.focus, m.modals.EditFocus)
			assert.Equal(t, "s1", m.modals.EditSessionID)
			assert.Equal(t, "alpha", m.modals.EditName.Value())
			assert.Equal(t, "auto, backend", m.modals.EditTags.Value())
			assert.True(t, m.modals.HasEditorFocus(m.state))
		})
	}
}

func TestEditSessionTabSwitchesFields(t *testing.T) {
	m := pressKey(t, newEditSessionTestModel(t), tea.KeyPressMsg{Code: 'R', Text: "R"})

	m = editSessionKey(t, m, "tab", tea.KeyTab)
	assert.Equal(t, editFieldTags, m.modals.EditFocus)
	assert.True(t, m.modals.EditTags.Focused())
	assert.False(t, m.modals.EditName.Focused())

	m = editSessionKey(t, m, "shift+tab", tea.KeyTab)
	assert.Equal(t, editFieldName, m.modals.EditFocus)
	assert.True(t, m.modals.EditName.Focused())
}

func TestEditSessionRequestHoldsOnlyChangedFields(t *testing.T) {
	tests := []struct {
		name        string
		nameValue   string
		tagsValue   string
		wantRename  bool
		wantSetTags bool
	}{
		{"nothing changed", "alpha", "auto, backend", false, false},
		{"spacing is not a change", " alpha ", " auto ,backend,", false, false},
		{"a new spelling is a change", "alpha", "Auto, backend", false, true},
		{"name only", "beta", "auto, backend", true, false},
		{"tags only", "alpha", "auto", false, true},
		{"both", "beta", "review", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := pressKey(t, newEditSessionTestModel(t), tea.KeyPressMsg{Code: 'R', Text: "R"})
			m.modals.EditName.SetValue(tt.nameValue)
			m.modals.EditTags.SetValue(tt.tagsValue)

			req, err := m.modals.editSessionRequest()
			require.NoError(t, err)
			assert.Equal(t, tt.wantRename, req.rename)
			assert.Equal(t, tt.wantSetTags, req.setTags)
			assert.Equal(t, "s1", req.sessionID)
		})
	}
}

func TestEditSessionKeepsInvalidInputOpen(t *testing.T) {
	tests := []struct {
		name      string
		nameValue string
		tagsValue string
	}{
		{"tag too long", "alpha", "auto, " + strings.Repeat("x", session.MaxTagLength+1)},
		{"empty name", "  ", "auto"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := pressKey(t, newEditSessionTestModel(t), tea.KeyPressMsg{Code: 'R', Text: "R"})
			m.modals.EditName.SetValue(tt.nameValue)
			m.modals.EditTags.SetValue(tt.tagsValue)

			m = editSessionKey(t, m, keyEnter, tea.KeyEnter)

			assert.Equal(t, stateEditingSession, m.state, "the user fixes the text rather than retyping it")
			assert.Equal(t, "s1", m.modals.EditSessionID)
		})
	}
}

func TestEditSessionEnterSavesAndUnchangedEnterOnlyCloses(t *testing.T) {
	m := pressKey(t, newEditSessionTestModel(t), tea.KeyPressMsg{Code: 'R', Text: "R"})
	model, cmd := m.handleEditSessionKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}), keyEnter)
	m = model.(Model)
	assert.Equal(t, stateNormal, m.state)
	assert.Nil(t, cmd, "an unchanged session writes nothing")

	m = pressKey(t, newEditSessionTestModel(t), tea.KeyPressMsg{Code: 'R', Text: "R"})
	m.modals.EditTags.SetValue("review")
	model, cmd = m.handleEditSessionKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}), keyEnter)
	m = model.(Model)
	assert.Equal(t, stateNormal, m.state)
	assert.Empty(t, m.modals.EditSessionID)
	require.NotNil(t, cmd)
}

func TestExecuteEditSessionSkipsTagsWhenTheRenameFails(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	sessionStore := store.NewSessionStore(database)
	now := time.Now()
	for _, sess := range []session.Session{
		{ID: "s1", Name: "alpha", Slug: "alpha", Path: "/tmp/alpha", State: session.StateActive, Tags: []string{"auto"}, CreatedAt: now, UpdatedAt: now},
		{ID: "s2", Name: "beta", Slug: "beta", Path: "/tmp/beta", State: session.StateActive, CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, sessionStore.Save(t.Context(), sess))
	}
	m := newEditSessionTestModel(t)
	m.service = newTestSessionService(t, sessionStore)

	msg := m.executeEditSession(editSessionRequest{
		sessionID: "s1", name: "beta", rename: true, tags: []string{"review"}, setTags: true,
	})()

	completed, ok := msg.(editSessionCompleteMsg)
	require.True(t, ok)
	require.Error(t, completed.err, "beta is taken")
	got, err := sessionStore.Get(t.Context(), "s1")
	require.NoError(t, err)
	assert.Equal(t, []string{"auto"}, got.Tags)
}
