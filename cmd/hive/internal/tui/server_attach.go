package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/cmd/hive/internal/serverattach"
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

// openServerNewSessionForm opens the form with the server's repositories and
// agents. A server's repository has no path here, so the create sends none.
func (m Model) openServerNewSessionForm(msg serverLaunchOptionsMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.notifyErrorf("read the repositories of server %s: %v", msg.server, msg.err)
		return m, nil
	}
	repos := make([]workspace.DiscoveredRepo, 0, len(msg.options.Repositories))
	for _, r := range msg.options.Repositories {
		repos = append(repos, workspace.DiscoveredRepo{Name: r.Name, Remote: r.Remote})
	}
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
			return actionCompleteMsg{err: err}
		}
		sess, err := api.CreateSession(context.Background(), sessionsvc.CreateOptions{
			Name: name, Remote: repo.Remote, Background: true, AgentKey: agentKey,
		})
		if err != nil {
			return actionCompleteMsg{err: fmt.Errorf("create session on server %s: %w", serverName, err)}
		}
		ref := session.Ref{Server: serverName, ID: sess.ID}
		return actionCompleteMsg{sessionID: ref.String(), message: fmt.Sprintf("Session %s created on %s", sess.Name, serverName)}
	}
}

// serverAttachTimeout bounds the start on the server, which may spawn the
// agent; the attach after it has no limit.
const serverAttachTimeout = 30 * time.Second

// serverAttachReadyMsg is a started server session the TUI attaches to next.
type serverAttachReadyMsg struct {
	sessionID   string
	window      string
	host        server.Host
	tmuxSession string
}

// startServerAttach starts the session on its server off the update loop,
// so a slow ssh never holds the keys.
func (m Model) startServerAttach(ref session.Ref) tea.Cmd {
	qualified := ref.String()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), serverAttachTimeout)
		defer cancel()
		sess, err := m.sessions.GetSession(ctx, qualified)
		if err != nil {
			return actionCompleteMsg{sessionID: qualified, err: err}
		}
		sess.ID = ref.ID
		host, err := m.serverAttach.StartServerSession(ctx, ref.Server, sess)
		if err != nil {
			return actionCompleteMsg{sessionID: qualified, err: err}
		}
		return serverAttachReadyMsg{
			sessionID:   qualified,
			window:      ref.Server + "/" + sess.Name,
			host:        host,
			tmuxSession: sessionsvc.Target(sess).Session,
		}
	}
}

// attachServer opens the session in a new tmux window when the TUI runs in
// tmux, so the TUI stays on screen. Outside tmux, ssh takes the terminal
// until the user detaches.
func (m Model) attachServer(msg serverAttachReadyMsg) tea.Cmd {
	if os.Getenv("TMUX") != "" {
		return func() tea.Msg {
			err := serverattach.OpenWindow(context.Background(), msg.host, msg.tmuxSession, msg.window)
			return actionCompleteMsg{sessionID: msg.sessionID, err: err}
		}
	}
	argv := serverattach.Command(msg.host, msg.tmuxSession)
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec,noctx // argv is built from the config's ssh target; the user ends it by detaching
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return actionCompleteMsg{sessionID: msg.sessionID, err: err}
	})
}
