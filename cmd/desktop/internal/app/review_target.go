package app

import (
	"context"

	"golang.org/x/sync/errgroup"

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
	AgentPanes(ctx context.Context, sessions []*session.Session) map[string][]statussvc.AgentPane
}

// serverAgents reaches the agents of a session on a server: its panes, read
// by the server's daemon, and the server's tmux to type into them.
type serverAgents interface {
	SessionAgentPanes(ctx context.Context, server, id string) (session.Session, []statussvc.AgentPane, error)
	Input(ctx context.Context, server string) (agentInput, error)
}

type engineCodeAgents struct{ engine *hive.Engine }

func (e engineCodeAgents) ListSessions(ctx context.Context) ([]session.Session, error) {
	return e.engine.Sessions().ListSessions(ctx)
}

func (e engineCodeAgents) AgentPanes(ctx context.Context, sessions []*session.Session) map[string][]statussvc.AgentPane {
	return e.engine.Status().AgentPanes(ctx, sessions)
}

// resolvedTarget is a target with the pane Send delivers to, which a chat's
// ref leaves out.
type resolvedTarget struct {
	AgentTarget
	deliverTo string
	// server is the server whose tmux holds deliverTo, or "" for this machine.
	server string
}

// AgentTargets lists the agents that can take text: classified agent panes in
// active Code sessions, and running chats. A pane with no detected tool or no
// status may be a shell, which would run the feedback as commands, so it is
// left out.
func (s *ReviewService) AgentTargets(ctx context.Context) ([]AgentTarget, error) {
	var code, chats []resolvedTarget
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() (err error) {
		code, err = s.codeTargets(groupCtx, "")
		return err
	})
	group.Go(func() (err error) {
		chats, err = s.chatTargets(groupCtx, "")
		return err
	})
	if err := group.Wait(); err != nil {
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
// session id when it is set. An id that names a server lists that session's
// panes on the server; the list of every session stays on this machine.
func (s *ReviewService) codeTargets(ctx context.Context, id string) ([]resolvedTarget, error) {
	if ref, err := session.ParseRef(id, session.LocalServer); id != "" && err == nil && ref.Server != session.LocalServer {
		return s.serverCodeTargets(ctx, id, ref)
	}
	if s.code == nil {
		return nil, nil
	}
	sessions, err := s.code.ListSessions(ctx)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing sessions")
	}
	var active []*session.Session
	for i := range sessions {
		if sessions[i].State == session.StateActive && (id == "" || sessions[i].ID == id) {
			active = append(active, &sessions[i])
		}
	}
	panes := s.code.AgentPanes(ctx, active)

	var targets []resolvedTarget
	for _, sess := range active {
		targets = append(targets, paneTargets(sess, sess.ID, panes[sess.ID])...)
	}
	disambiguateLabels(targets)
	return targets, nil
}

func (s *ReviewService) serverCodeTargets(ctx context.Context, id string, ref session.Ref) ([]resolvedTarget, error) {
	if s.servers == nil {
		return nil, Errorf(KindUnavailable, "sending to an agent on a server is not available")
	}
	sess, panes, err := s.servers.SessionAgentPanes(ctx, ref.Server, ref.ID)
	if err != nil {
		return nil, err
	}
	if sess.State != session.StateActive {
		return nil, nil
	}
	targets := paneTargets(&sess, id, panes)
	for i := range targets {
		targets[i].server = ref.Server
	}
	disambiguateLabels(targets)
	return targets, nil
}

// paneTargets turns a session's agent panes into targets under id, the id the
// frontend knows the session by.
func paneTargets(sess *session.Session, id string, panes []statussvc.AgentPane) []resolvedTarget {
	var targets []resolvedTarget
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
			ID:        id,
			Pane:      pane.PaneID,
			Label:     label,
			Status:    string(status),
			deliverTo: pane.PaneID,
		})
	}
	return targets
}

// disambiguateLabels names the pane where two agents would otherwise read the
// same: a session often runs two windows both called after the tool.
func disambiguateLabels(targets []resolvedTarget) {
	seen := make(map[string]int, len(targets))
	for _, t := range targets {
		seen[t.Label]++
	}
	for i := range targets {
		if seen[targets[i].Label] > 1 {
			targets[i].Label += " (" + targets[i].Pane + ")"
		}
	}
}

// chatTargets lists running chats, or the one chat id when it is set.
func (s *ReviewService) chatTargets(ctx context.Context, id string) ([]resolvedTarget, error) {
	if s.chats == nil {
		return nil, nil
	}
	chats, err := s.chats.RunningChats(ctx)
	if err != nil {
		return nil, err
	}
	var targets []resolvedTarget
	for _, chat := range chats {
		status := chat.Status.Simplified()
		if (id != "" && chat.ID != id) || !sendable(status) {
			continue
		}
		targets = append(targets, resolvedTarget{
			Kind:      AgentTargetChat,
			ID:        chat.ID,
			Label:     chat.Workspace + " / " + chat.Name,
			Status:    string(status),
			deliverTo: chat.Pane,
		})
	}
	return targets, nil
}

func sendable(status terminal.Status) bool {
	return status == terminal.StatusReady || status == terminal.StatusActive || status == terminal.StatusApproval
}
