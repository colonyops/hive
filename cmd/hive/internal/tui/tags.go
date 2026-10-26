package tui

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/cmd/hive/internal/styles"
	"github.com/colonyops/hive/cmd/hive/internal/tui/views/sessions"
	"github.com/colonyops/hive/internal/domain/session"
)

type setTagsCompleteMsg struct {
	err error
}

func (m Model) openTagsInput(sess *session.Session) (tea.Model, tea.Cmd) {
	input := textinput.New()
	input.SetValue(strings.Join(sess.Tags, ", "))
	input.Focus()
	input.CharLimit = 512
	input.Prompt = ""
	input.Placeholder = "auto, backend"
	input.SetWidth(50)
	input.KeyMap.Paste.SetEnabled(true)
	inputStyles := textinput.DefaultStyles(true)
	inputStyles.Cursor.Color = styles.ColorPrimary
	input.SetStyles(inputStyles)

	m.modals.TagsInput = input
	m.modals.TagsSessionID = sess.ID
	m.state = stateEditingTags
	return m, nil
}

// handleTagsKey keeps the modal open on an invalid tag, so the user can
// fix the text instead of typing it again.
func (m Model) handleTagsKey(msg tea.KeyPressMsg, keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyCtrlC:
		return m.quit()
	case "esc":
		m.state = stateNormal
		m.modals.TagsSessionID = ""
		return m, nil
	case keyEnter:
		tags, err := session.ParseTags(m.modals.TagsInput.Value())
		if err != nil {
			return m, m.notifyError("invalid tags: %v", err)
		}
		sessionID := m.modals.TagsSessionID
		m.state = stateNormal
		m.modals.TagsSessionID = ""
		return m, m.executeSetTags(sessionID, tags)
	}

	var cmd tea.Cmd
	m.modals.TagsInput, cmd = m.modals.TagsInput.Update(msg)
	return m, cmd
}

func (m Model) executeSetTags(sessionID string, tags []string) tea.Cmd {
	return func() tea.Msg {
		_, err := m.service.SetSessionTags(context.Background(), sessionID, tags)
		return setTagsCompleteMsg{err: err}
	}
}

func (m Model) handleSetTagsComplete(msg setTagsCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.logger.Error().Err(msg.err).Msg("set tags failed")
		return m, m.notifyError("set tags failed: %v", msg.err)
	}
	return m, func() tea.Msg { return sessions.RefreshSessionsMsg{} }
}
