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

func newTagsTestModel(t *testing.T) Model {
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

// pressTagsKey sends ctrl+t and delivers the action request the sessions view
// returns for it.
func pressTagsKey(t *testing.T, m Model) Model {
	t.Helper()
	model, cmd := m.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	m = model.(Model)
	require.NotNil(t, cmd)
	model, _ = m.Update(cmd())
	return model.(Model)
}

func TestTagsKeyOpensEditorWithCurrentTags(t *testing.T) {
	m := newTagsTestModel(t)

	m = pressTagsKey(t, m)

	assert.Equal(t, stateEditingTags, m.state)
	assert.Equal(t, "s1", m.modals.TagsSessionID)
	assert.Equal(t, "auto, backend", m.modals.TagsInput.Value())
	assert.True(t, m.modals.HasEditorFocus(m.state))
}

func TestTagsEditorKeepsInvalidInputOpen(t *testing.T) {
	m := newTagsTestModel(t)
	m = pressTagsKey(t, m)
	m.modals.TagsInput.SetValue("auto, " + strings.Repeat("x", session.MaxTagLength+1))

	model, _ := m.handleTagsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}), keyEnter)
	m = model.(Model)

	assert.Equal(t, stateEditingTags, m.state, "the user fixes the text rather than retyping it")
	assert.Equal(t, "s1", m.modals.TagsSessionID)
}

func TestTagsEditorSubmitsParsedTags(t *testing.T) {
	m := newTagsTestModel(t)
	m = pressTagsKey(t, m)
	m.modals.TagsInput.SetValue("auto, , Auto, review")

	model, cmd := m.handleTagsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}), keyEnter)
	m = model.(Model)

	assert.Equal(t, stateNormal, m.state)
	assert.Empty(t, m.modals.TagsSessionID)
	require.NotNil(t, cmd)
	assert.IsType(t, setTagsCompleteMsg{}, cmd())
}

func TestTagsEditorEscCancels(t *testing.T) {
	m := newTagsTestModel(t)
	m = pressTagsKey(t, m)

	model, cmd := m.handleTagsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}), "esc")
	m = model.(Model)

	assert.Equal(t, stateNormal, m.state)
	assert.Nil(t, cmd)
}
