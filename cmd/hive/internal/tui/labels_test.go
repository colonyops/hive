package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/hive/internal/tui/views/sessions"
	"github.com/colonyops/hive/internal/domain/session"
)

func newLabelsTestModel(t *testing.T) Model {
	t.Helper()
	m := newKeybindingPrecedenceModel(t, nil)
	m = switchTestView(t, m, ViewSessions)
	item := sessions.TreeItem{Session: session.Session{
		ID: "s1", Name: "alpha", State: session.StateActive, Labels: []string{"auto", "backend"},
	}}
	listModel := getUnexportedField[list.Model](t, m.sessionsView, "list")
	listModel.SetItems([]list.Item{item})
	setUnexportedField(t, m.sessionsView, "list", listModel)
	return m
}

// pressLabelsKey sends L and delivers the action request the sessions view
// returns for it.
func pressLabelsKey(t *testing.T, m Model) Model {
	t.Helper()
	model, cmd := m.Update(keyPressMsg("L"))
	m = model.(Model)
	require.NotNil(t, cmd)
	model, _ = m.Update(cmd())
	return model.(Model)
}

func TestLabelsKeyOpensEditorWithCurrentLabels(t *testing.T) {
	m := newLabelsTestModel(t)

	m = pressLabelsKey(t, m)

	assert.Equal(t, stateEditingLabels, m.state)
	assert.Equal(t, "s1", m.modals.LabelsSessionID)
	assert.Equal(t, "auto, backend", m.modals.LabelsInput.Value())
	assert.True(t, m.modals.HasEditorFocus(m.state))
}

func TestLabelsEditorKeepsInvalidInputOpen(t *testing.T) {
	m := newLabelsTestModel(t)
	m = pressLabelsKey(t, m)
	m.modals.LabelsInput.SetValue("auto, " + strings.Repeat("x", session.MaxLabelLength+1))

	model, _ := m.handleLabelsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}), keyEnter)
	m = model.(Model)

	assert.Equal(t, stateEditingLabels, m.state, "the user fixes the text rather than retyping it")
	assert.Equal(t, "s1", m.modals.LabelsSessionID)
}

func TestLabelsEditorSubmitsParsedLabels(t *testing.T) {
	m := newLabelsTestModel(t)
	m = pressLabelsKey(t, m)
	m.modals.LabelsInput.SetValue("auto, , Auto, review")

	model, cmd := m.handleLabelsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}), keyEnter)
	m = model.(Model)

	assert.Equal(t, stateNormal, m.state)
	assert.Empty(t, m.modals.LabelsSessionID)
	require.NotNil(t, cmd)
	assert.IsType(t, setLabelsCompleteMsg{}, cmd())
}

func TestLabelsEditorEscCancels(t *testing.T) {
	m := newLabelsTestModel(t)
	m = pressLabelsKey(t, m)

	model, cmd := m.handleLabelsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}), "esc")
	m = model.(Model)

	assert.Equal(t, stateNormal, m.state)
	assert.Nil(t, cmd)
}
