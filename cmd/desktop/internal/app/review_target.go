package app

import (
	"context"
	"slices"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive"
	statussvc "github.com/colonyops/hive/internal/hive/status"
	"github.com/colonyops/hive/internal/platform/git"
)

const (
	AgentTargetSession = "session"
	AgentTargetChat    = "chat"
)

// AgentTargetRef names one agent that can take text. The frontend sends it
// back as it got it; Send resolves it again and never trusts it as a tmux
// target.
type AgentTargetRef struct {
	Kind string `json:"kind"` // AgentTargetSession or AgentTargetChat
	ID   string `json:"id"`   // hive session id or chat id
	Pane string `json:"pane"` // tmux pane id; session targets only
}

type AgentTarget struct {
	AgentTargetRef
	Label  string `json:"label"`  // "<repo> / <session> · <window>" or "<workspace> / <chat>"
	Status string `json:"status"` // ready | active | approval
}

type codeAgents interface {
	ListSessions(ctx context.Context) ([]session.Session, error)
	AgentPanes(ctx context.Context, sess *session.Session) ([]statussvc.AgentPane, error)
}

type engineCodeAgents struct{ engine *hive.Engine }

func (e engineCodeAgents) ListSessions(ctx context.Context) ([]session.Session, error) {
	return e.engine.Sessions().ListSessions(ctx)
}

func (e engineCodeAgents) AgentPanes(ctx context.Context, sess *session.Session) ([]statussvc.AgentPane, error) {
	return e.engine.Status().AgentPanes(ctx, sess)
}

// resolvedTarget is a target with the pane Send delivers to, which a chat's
// ref leaves out.
type resolvedTarget struct {
	AgentTarget
	deliverTo string
}

// AgentTargets lists the agents that can take text: classified agent panes in
// active Code sessions, and running chats. A pane with no detected tool or no
// status may be a shell, which would run the feedback as commands, so it is
// left out.
func (s *ReviewService) AgentTargets(ctx context.Context) ([]AgentTarget, error) {
	code, err := s.codeTargets(ctx, "")
	if err != nil {
		return nil, err
	}
	chats, err := s.chatTargets(ctx, "")
	if err != nil {
		return nil, err
	}
	targets := make([]AgentTarget, 0, len(code)+len(chats))
	for _, t := range append(code, chats...) {
		targets = append(targets, t.AgentTarget)
	}
	return targets, nil
}

func (s *ReviewService) resolveTarget(ctx context.Context, ref AgentTargetRef) (resolvedTarget, error) {
	var (
		targets []resolvedTarget
		err     error
	)
	switch ref.Kind {
	case AgentTargetSession:
		targets, err = s.codeTargets(ctx, ref.ID)
	case AgentTargetChat:
		targets, err = s.chatTargets(ctx, ref.ID)
	default:
		return resolvedTarget{}, Errorf(KindInvalid, "%q is not an agent target kind", ref.Kind)
	}
	if err != nil {
		return resolvedTarget{}, err
	}
	for _, t := range targets {
		if ref.Kind == AgentTargetChat || t.Pane == ref.Pane {
			return t, nil
		}
	}
	return resolvedTarget{}, Errorf(KindNotFound, "that agent is no longer running")
}

// codeTargets lists the agent panes of active Code sessions, or of the one
// session id when it is set.
func (s *ReviewService) codeTargets(ctx context.Context, id string) ([]resolvedTarget, error) {
	if s.code == nil {
		return nil, nil
	}
	sessions, err := s.code.ListSessions(ctx)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing sessions")
	}
	var targets []resolvedTarget
	for i := range sessions {
		sess := &sessions[i]
		if sess.State != session.StateActive || (id != "" && sess.ID != id) {
			continue
		}
		panes, err := s.code.AgentPanes(ctx, sess)
		if err != nil {
			s.log.Debug().Ctx(ctx).Err(err).Str("session", sess.ID).Msg("agent pane discovery failed")
			continue
		}
		for _, pane := range panes {
			status := pane.Status.Simplified()
			if pane.Tool == "" || !sendable(status) {
				continue
			}
			label := sess.Name
			if repo := git.ExtractRepoName(sess.Remote); repo != "" {
				label = repo + " / " + label
			}
			if pane.WindowName != "" {
				label += " · " + pane.WindowName
			}
			targets = append(targets, resolvedTarget{
				Kind:      AgentTargetSession,
				ID:        sess.ID,
				Pane:      pane.PaneID,
				Label:     label,
				Status:    string(status),
				deliverTo: pane.PaneID,
			})
		}
	}
	return targets, nil
}

// chatTargets lists running chats, or the one chat id when it is set. The
// agent is the first pane of the chat's tmux session, where it was launched.
func (s *ReviewService) chatTargets(ctx context.Context, id string) ([]resolvedTarget, error) {
	if s.chats == nil || s.input == nil {
		return nil, nil
	}
	chats, err := s.chats.AllSessions(ctx)
	if err != nil {
		return nil, err
	}
	chats = slices.DeleteFunc(chats, func(c SessionView) bool {
		return c.TerminalID == "" || (id != "" && c.ID != id)
	})
	if len(chats) == 0 {
		return nil, nil
	}
	activity, err := s.chats.SessionActivity(ctx, "")
	if err != nil {
		return nil, err
	}
	statuses := make(map[string]terminal.Status, len(activity))
	for _, a := range activity {
		statuses[a.ID] = terminal.Status(a.Status).Simplified()
	}

	var targets []resolvedTarget
	for _, chat := range chats {
		status, ok := statuses[chat.ID]
		if !ok || !sendable(status) {
			continue
		}
		pane, err := s.input.FirstPane(ctx, chat.TerminalID)
		if err != nil {
			s.log.Debug().Ctx(ctx).Err(err).Str("chat", chat.ID).Msg("chat pane lookup failed")
			continue
		}
		targets = append(targets, resolvedTarget{
			Kind:      AgentTargetChat,
			ID:        chat.ID,
			Label:     chat.Workspace + " / " + chat.Name,
			Status:    string(status),
			deliverTo: pane,
		})
	}
	return targets, nil
}

func sendable(status terminal.Status) bool {
	return status == terminal.StatusReady || status == terminal.StatusActive || status == terminal.StatusApproval
}
