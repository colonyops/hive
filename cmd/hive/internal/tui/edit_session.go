package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/colonyops/hive/cmd/hive/internal/styles"
	"github.com/colonyops/hive/cmd/hive/internal/tui/components"
	"github.com/colonyops/hive/cmd/hive/internal/tui/views/sessions"
	"github.com/colonyops/hive/internal/domain/session"
)

type editSessionField int

const (
	editFieldName editSessionField = iota
	editFieldTags
)

type editSessionCompleteMsg struct {
	err error
}

// editSessionRequest holds only what the user changed, so an unchanged name
// never renames the tmux session.
type editSessionRequest struct {
	sessionID string
	name      string
	rename    bool
	tags      []string
	setTags   bool
}

func (r editSessionRequest) empty() bool { return !r.rename && !r.setTags }

func newEditSessionInput(value, placeholder string, limit int) textinput.Model {
	input := textinput.New()
	input.SetValue(value)
	input.CharLimit = limit
	input.Prompt = ""
	input.Placeholder = placeholder
	input.SetWidth(50)
	input.KeyMap.Paste.SetEnabled(true)
	inputStyles := textinput.DefaultStyles(true)
	inputStyles.Cursor.Color = styles.ColorPrimary
	input.SetStyles(inputStyles)
	return input
}

func (m Model) openEditSession(sess *session.Session, focus editSessionField) (tea.Model, tea.Cmd) {
	m.modals.EditName = newEditSessionInput(sess.Name, "", session.MaxNameLength)
	m.modals.EditTags = newEditSessionInput(strings.Join(sess.Tags, ", "), "auto, backend", 512)
	m.modals.EditSessionID = sess.ID
	m.modals.EditOrigName = sess.Name
	m.modals.EditOrigTags = slices.Clone(sess.Tags)
	m.modals.focusEditField(focus)
	m.state = stateEditingSession
	return m, nil
}

func (mc *ModalCoordinator) focusEditField(field editSessionField) {
	mc.EditFocus = field
	if field == editFieldName {
		mc.EditName.Focus()
		mc.EditTags.Blur()
		return
	}
	mc.EditTags.Focus()
	mc.EditName.Blur()
}

// editSessionRequest validates both fields. An error keeps the modal open,
// so the user fixes the text instead of typing it again.
func (mc *ModalCoordinator) editSessionRequest() (editSessionRequest, error) {
	name := strings.TrimSpace(mc.EditName.Value())
	if err := session.ValidateName(name); err != nil {
		return editSessionRequest{}, err
	}
	tags, err := session.ParseTags(mc.EditTags.Value())
	if err != nil {
		return editSessionRequest{}, err
	}
	// Tags stored before validation existed may not normalize; compare them
	// as stored, so saving them unchanged is still a no-op.
	original, err := session.NormalizeTags(mc.EditOrigTags)
	if err != nil {
		original = mc.EditOrigTags
	}
	return editSessionRequest{
		sessionID: mc.EditSessionID,
		name:      name,
		rename:    name != mc.EditOrigName,
		tags:      tags,
		setTags:   !slices.Equal(tags, original),
	}, nil
}

func (m Model) handleEditSessionKey(msg tea.KeyPressMsg, keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyCtrlC:
		return m.quit()
	case "esc":
		m.state = stateNormal
		m.modals.EditSessionID = ""
		return m, nil
	case "tab", "shift+tab", "up", "down":
		if m.modals.EditFocus == editFieldName {
			m.modals.focusEditField(editFieldTags)
		} else {
			m.modals.focusEditField(editFieldName)
		}
		return m, nil
	case keyEnter:
		req, err := m.modals.editSessionRequest()
		if err != nil {
			return m, m.notifyError("invalid session: %v", err)
		}
		m.state = stateNormal
		m.modals.EditSessionID = ""
		if req.empty() {
			return m, nil
		}
		return m, m.executeEditSession(req)
	}

	var cmd tea.Cmd
	if m.modals.EditFocus == editFieldName {
		m.modals.EditName, cmd = m.modals.EditName.Update(msg)
	} else {
		m.modals.EditTags, cmd = m.modals.EditTags.Update(msg)
	}
	return m, cmd
}

// executeEditSession writes tags only after the rename succeeds, so a failed
// rename leaves the session as it was.
func (m Model) executeEditSession(req editSessionRequest) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if req.rename {
			if err := m.service.RenameSession(ctx, req.sessionID, req.name); err != nil {
				return editSessionCompleteMsg{err: fmt.Errorf("rename: %w", err)}
			}
		}
		if req.setTags {
			if _, err := m.service.SetSessionTags(ctx, req.sessionID, req.tags); err != nil {
				return editSessionCompleteMsg{err: fmt.Errorf("set tags: %w", err)}
			}
		}
		return editSessionCompleteMsg{}
	}
}

func (mc *ModalCoordinator) editSessionView() string {
	label := func(text string, focused bool) string {
		if focused {
			return styles.ModalTitleStyle.Render(text)
		}
		return styles.ModalHelpStyle.Render(text)
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		styles.ModalTitleStyle.Render("Edit Session"),
		"",
		label("Name", mc.EditFocus == editFieldName),
		mc.EditName.View(),
		"",
		label("Tags (comma separated)", mc.EditFocus == editFieldTags),
		mc.EditTags.View(),
		"",
		styles.ModalHelpStyle.Render(components.KeyHints(
			components.HelpEntry{Key: "tab", Desc: "next field"},
			components.HelpEntry{Key: "enter", Desc: "save"},
			components.HelpEntry{Key: "esc", Desc: "cancel"},
		)),
	)
}

func (m Model) handleEditSessionComplete(msg editSessionCompleteMsg) (tea.Model, tea.Cmd) {
	refresh := func() tea.Msg { return sessions.RefreshSessionsMsg{} }
	if msg.err != nil {
		m.logger.Error().Err(msg.err).Msg("edit session failed")
		// A rename can succeed before the tag write fails, so refresh anyway.
		return m, tea.Batch(m.notifyError("edit session failed: %v", msg.err), refresh)
	}
	return m, refresh
}
