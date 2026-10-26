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

type setLabelsCompleteMsg struct {
	err error
}

func (m Model) openLabelsInput(sess *session.Session) (tea.Model, tea.Cmd) {
	input := textinput.New()
	input.SetValue(strings.Join(sess.Labels, ", "))
	input.Focus()
	input.CharLimit = 512
	input.Prompt = ""
	input.Placeholder = "auto, backend"
	input.SetWidth(50)
	input.KeyMap.Paste.SetEnabled(true)
	inputStyles := textinput.DefaultStyles(true)
	inputStyles.Cursor.Color = styles.ColorPrimary
	input.SetStyles(inputStyles)

	m.modals.LabelsInput = input
	m.modals.LabelsSessionID = sess.ID
	m.state = stateEditingLabels
	return m, nil
}

// handleLabelsKey keeps the modal open on an invalid label, so the user can
// fix the text instead of typing it again.
func (m Model) handleLabelsKey(msg tea.KeyPressMsg, keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyCtrlC:
		return m.quit()
	case "esc":
		m.state = stateNormal
		m.modals.LabelsSessionID = ""
		return m, nil
	case keyEnter:
		labels, err := session.ParseLabels(m.modals.LabelsInput.Value())
		if err != nil {
			return m, m.notifyError("invalid labels: %v", err)
		}
		sessionID := m.modals.LabelsSessionID
		m.state = stateNormal
		m.modals.LabelsSessionID = ""
		return m, m.executeSetLabels(sessionID, labels)
	}

	var cmd tea.Cmd
	m.modals.LabelsInput, cmd = m.modals.LabelsInput.Update(msg)
	return m, cmd
}

func (m Model) executeSetLabels(sessionID string, labels []string) tea.Cmd {
	return func() tea.Msg {
		_, err := m.service.SetSessionLabels(context.Background(), sessionID, labels)
		return setLabelsCompleteMsg{err: err}
	}
}

func (m Model) handleSetLabelsComplete(msg setLabelsCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.logger.Error().Err(msg.err).Msg("set labels failed")
		return m, m.notifyError("set labels failed: %v", msg.err)
	}
	return m, func() tea.Msg { return sessions.RefreshSessionsMsg{} }
}
