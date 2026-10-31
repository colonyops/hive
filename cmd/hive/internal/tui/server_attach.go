package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/cmd/hive/internal/serverattach"
	"github.com/colonyops/hive/internal/domain/notify"
	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/domain/session"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
	"github.com/colonyops/hive/internal/platform/workspace"

	"github.com/colonyops/hive/cmd/hive/internal/tui/views/sessions"
)

// ServerAttacher reaches the servers under servers:. StartServerSession
// starts a server session's terminal on the server and returns the ssh
// target to attach with.
type ServerAttacher interface {
	StartServerSession(ctx context.Context, serverName string, sess session.Session) (server.Host, error)
	ServerAPI(name string) (sessionapi.API, error)
}

// serverLaunchOptionsMsg carries a server's repositories and agents for the
// new session form.
type serverLaunchOptionsMsg struct {
	server      string
	preselected string
	options     sessionsvc.LaunchOptions
	err         error
}

func (m Model) loadServerLaunchOptions(name, preselected string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), serverAttachTimeout)
		defer cancel()
		api, err := m.serverAttach.ServerAPI(name)
		if err != nil {
			return serverLaunchOptionsMsg{server: name, err: err}
		}
		options, err := api.SessionLaunchOptions(ctx)
		return serverLaunchOptionsMsg{server: name, preselected: preselected, options: options, err: err}
	}
}

// serverActionDoneMsg ends work that ran in the background with the keys
// free, so the user may have opened a modal since it started.
type serverActionDoneMsg struct {
	actionCompleteMsg
}

// handleServerActionDone keeps a modal the user opened during the work. A
// modal holds the state and the pending action that handleActionComplete
// clears.
func (m Model) handleServerActionDone(msg serverActionDoneMsg) (tea.Model, tea.Cmd) {
	if m.state == stateNormal {
		return m.handleActionComplete(msg.actionCompleteMsg)
	}
	switch {
	case msg.err != nil:
		m.logger.Error().Err(msg.err).Msg("action failed")
		m.notifyErrorf("action failed: %v", msg.err)
	case msg.message != "":
		m.publishNotificationf(notify.LevelInfo, "%s", msg.message)
	}
	return m, m.refreshSessions()
}

// serverRepos keeps each repository's path, which is on the server: the server
// copies rule files from it.
func serverRepos(options sessionsvc.LaunchOptions) []workspace.DiscoveredRepo {
	repos := make([]workspace.DiscoveredRepo, 0, len(options.Repositories))
	for _, r := range options.Repositories {
		repos = append(repos, workspace.DiscoveredRepo{Name: r.Name, Remote: r.Remote, Path: r.Source})
	}
	return repos
}

// openServerNewSessionForm opens the form with the server's repositories and
// agents.
func (m Model) openServerNewSessionForm(msg serverLaunchOptionsMsg) (tea.Model, tea.Cmd) {
	if m.state != stateNormal {
		m.logger.Debug().Str("server", msg.server).Msg("a modal opened while the server's repositories loaded; not opening the form")
		return m, nil
	}
	if msg.err != nil {
		m.notifyErrorf("read the repositories of server %s: %v", msg.server, msg.err)
		return m, nil
	}
	repos := serverRepos(msg.options)
	if len(repos) == 0 {
		m.notifyErrorf("server %s has no repositories to create a session from", msg.server)
		return m, nil
	}
	preselected := msg.preselected
	if preselected == "" {
		preselected = msg.options.DefaultRepository
	}
	var agentKeys []string
	if len(msg.options.Agents) > 1 {
		agentKeys = append(agentKeys, msg.options.DefaultAgent)
		for _, k := range msg.options.Agents {
			if k != msg.options.DefaultAgent {
				agentKeys = append(agentKeys, k)
			}
		}
	}
	existing := map[string]bool{}
	for _, s := range m.sessionsView.AllSessions() {
		if sessions.ServerOf(s) == msg.server {
			existing[session.Slugify(s.Name)] = true
		}
	}
	form := NewNewSessionForm(repos, preselected, existing, agentKeys)
	form.server = msg.server
	m.modals.NewSession = form
	m.state = stateCreatingSession
	return m, form.Init()
}

// createOnServer creates the session with the server's own rules, in the
// background: the TUI attaches to it like any other row afterwards.
func (m Model) createOnServer(name string, repo workspace.DiscoveredRepo, agentKey, serverName string) tea.Cmd {
	return func() tea.Msg {
		api, err := m.serverAttach.ServerAPI(serverName)
		if err != nil {
			return serverActionDoneMsg{actionCompleteMsg{err: err}}
		}
		sess, err := api.CreateSession(context.Background(), sessionsvc.CreateOptions{
			Name: name, Remote: repo.Remote, Source: repo.Path, Background: true, AgentKey: agentKey,
		})
		if err != nil {
			return serverActionDoneMsg{actionCompleteMsg{err: fmt.Errorf("create session on server %s: %w", serverName, err)}}
		}
		ref := session.Ref{Server: serverName, ID: sess.ID}
		return serverActionDoneMsg{actionCompleteMsg{sessionID: ref.String(), message: fmt.Sprintf("Session %s created on %s", sess.Name, serverName)}}
	}
}

// serverAttachTimeout bounds the start on the server, which may spawn the
// agent; the attach after it has no limit.
const serverAttachTimeout = 30 * time.Second

// serverAttachReadyMsg is a started server session the TUI attaches to next.
type serverAttachReadyMsg struct {
	sessionID  string
	windowName string
	host       server.Host
	target     string
	exit       bool
}

// serverAttachExitMsg ends the TUI after an attach for a command with exit set.
type serverAttachExitMsg struct{}

// startServerAttach starts the session on its server off the update loop,
// so a slow ssh never holds the keys.
func (m Model) startServerAttach(ref session.Ref, action Action) tea.Cmd {
	qualified := ref.String()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), serverAttachTimeout)
		defer cancel()
		sess, err := m.sessions.GetSession(ctx, qualified)
		if err != nil {
			return serverActionDoneMsg{actionCompleteMsg{sessionID: qualified, err: err}}
		}
		sess.ID = ref.ID
		host, err := m.serverAttach.StartServerSession(ctx, ref.Server, sess)
		if errors.Is(err, serverattach.ErrSessionCompleted) {
			return serverActionDoneMsg{actionCompleteMsg{sessionID: qualified, message: err.Error()}}
		}
		if err != nil {
			return serverActionDoneMsg{actionCompleteMsg{sessionID: qualified, err: err}}
		}
		return serverAttachReadyMsg{
			sessionID:  qualified,
			windowName: ref.Server + "/" + sess.Name,
			host:       host,
			target:     serverattach.Target(sessionsvc.Target(sess).Session, action.TmuxWindow),
			exit:       action.Exit,
		}
	}
}

// attachServer opens the session in a new tmux window when the TUI runs in
// tmux, so the TUI stays on screen. Outside tmux, ssh takes the terminal
// until the user detaches.
func (m Model) attachServer(msg serverAttachReadyMsg) tea.Cmd {
	done := func(err error) tea.Msg {
		if err == nil && msg.exit {
			return serverAttachExitMsg{}
		}
		return serverActionDoneMsg{actionCompleteMsg{sessionID: msg.sessionID, err: err}}
	}
	if os.Getenv("TMUX") != "" {
		return func() tea.Msg {
			return done(serverattach.OpenWindow(context.Background(), msg.host, msg.target, msg.windowName))
		}
	}
	cmd, report := serverattach.Prepare(context.Background(), msg.host, msg.target)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return done(report(err)) })
}
