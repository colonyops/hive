package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/internal/platform/promptfile"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	"github.com/colonyops/hive/internal/hive/servers"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
)

// serverReadTimeout bounds one server's answer to a session list or a status
// poll, so the Code view shows an offline server as offline soon after it opens.
const serverReadTimeout = 5 * time.Second

// ServerSessionList is the sessions of one server under servers:, with the
// state this read found. A server that did not answer has no sessions and an
// Error.
type ServerSessionList struct {
	Server   string            `json:"server"`
	SSH      string            `json:"ssh"`
	State    server.State      `json:"state"`
	Error    string            `json:"error,omitempty"`
	Sessions []session.Session `json:"sessions"`
}

type ServerStatus struct {
	Name        string       `json:"name"`
	SSH         string       `json:"ssh"`
	State       server.State `json:"state"`
	Version     string       `json:"version,omitempty"`
	Error       string       `json:"error,omitempty"`
	RoundTripMs int64        `json:"roundTripMs"`
}

// ListServerSessions reads the sessions of every server at once, in config
// order. It is separate from ListSessions, so local sessions never wait on a
// server.
func (s *SessionsService) ListServerSessions(ctx context.Context) []ServerSessionList {
	fleet := s.fleet()
	if fleet == nil {
		return []ServerSessionList{}
	}
	all := fleet.Servers()
	out := make([]ServerSessionList, len(all))
	var wg sync.WaitGroup
	for i, srv := range all {
		wg.Go(func() { out[i] = listServerSessions(ctx, srv) })
	}
	wg.Wait()
	lists := make(map[string]ServerSessionList, len(out))
	for _, list := range out {
		lists[list.Server] = list
	}
	s.serverMu.Lock()
	s.serverLists = lists
	s.serverMu.Unlock()
	return out
}

// listedServers is what the last ListServerSessions found on the servers
// that answered it. A status poll asks only these, so an offline server costs
// one ssh attempt per list instead of one per poll.
func (s *SessionsService) listedServers() []ServerSessionList {
	s.serverMu.Lock()
	defer s.serverMu.Unlock()
	out := make([]ServerSessionList, 0, len(s.serverLists))
	for _, list := range s.serverLists {
		if list.State == server.StateConnected && len(list.Sessions) > 0 {
			out = append(out, list)
		}
	}
	return out
}

func listServerSessions(ctx context.Context, srv *servers.Server) ServerSessionList {
	ctx, cancel := context.WithTimeout(ctx, serverReadTimeout)
	defer cancel()
	cfg := srv.Config()
	list := ServerSessionList{Server: cfg.Name, SSH: cfg.SSH, State: server.StateConnected, Sessions: []session.Session{}}
	sessions, err := srv.Client().ListSessions(ctx)
	if err != nil {
		list.State = serverStateOf(err)
		list.Error = err.Error()
		return list
	}
	list.Sessions = sessions
	return list
}

func serverStateOf(err error) server.State {
	if _, ok := errors.AsType[*wire.IncompatibleError](err); ok {
		return server.StateIncompatible
	}
	return server.StateOffline
}

func (s *SessionsService) Servers(ctx context.Context) []ServerStatus {
	fleet := s.fleet()
	if fleet == nil {
		return []ServerStatus{}
	}
	checked := fleet.CheckAll(ctx)
	out := make([]ServerStatus, 0, len(checked))
	for _, st := range checked {
		out = append(out, ServerStatus{
			Name: st.Name, SSH: st.SSH, State: st.State, Version: st.Version,
			Error: st.Error, RoundTripMs: st.RoundTrip.Milliseconds(),
		})
	}
	return out
}

// sessionTarget is where a session id points: the session API and tmux of
// its server, and its id there.
type sessionTarget struct {
	api    sessionapi.API
	tmux   sessionTmux
	id     string
	server string
}

func (t sessionTarget) local() bool { return t.server == session.LocalServer }

// wrap classifies a failure of this target. A server that fails for any reason
// but a missing session is unreachable or refusing, not missing, whatever kind
// the same failure has on this machine.
func (t sessionTarget) wrap(err error, kind Kind, format string, args ...any) error {
	if t.local() || errors.Is(err, session.ErrNotFound) {
		return Wrap(err, kind, format, args...)
	}
	return serverError(err, KindUnavailable, format, args...)
}

// slugServer is the server a terminal slug names, if it names one.
func slugServer(slug string) (string, bool) {
	ref, err := session.ParseRef(slug, session.LocalServer)
	if err != nil || ref.Server == session.LocalServer {
		return "", false
	}
	return ref.Server, true
}

// target resolves an id from the Code view. "<server>:<id>" is a session on
// that server and a bare id is local, the form ParseRef reads.
func (s *SessionsService) target(ctx context.Context, id string) (sessionTarget, error) {
	ref, err := session.ParseRef(id, session.LocalServer)
	if err != nil {
		return sessionTarget{}, Wrap(err, KindInvalid, "session id")
	}
	if ref.Server == session.LocalServer {
		return sessionTarget{api: s.sessions, tmux: s.tmux, id: ref.ID, server: ref.Server}, nil
	}
	api, err := s.serverAPI(ref.Server)
	if err != nil {
		return sessionTarget{}, err
	}
	target := sessionTarget{api: api, id: ref.ID, server: ref.Server}
	if s.serverTmux != nil {
		tmux, err := s.serverTmux(ctx, ref.Server)
		if err != nil {
			return sessionTarget{}, err
		}
		target.tmux = tmux
	}
	return target, nil
}

func (s *SessionsService) serverAPI(name string) (sessionapi.API, error) {
	fleet := s.fleet()
	if fleet == nil {
		return nil, Errorf(KindUnavailable, "the server fleet is not loaded")
	}
	srv, ok := fleet.Server(name)
	if !ok {
		return nil, Errorf(KindNotFound, "server %q is not under servers: in config.yaml", name)
	}
	return srv.Client(), nil
}

func (s *SessionsService) fleet() *servers.Fleet {
	if s.servers == nil {
		return nil
	}
	return s.servers()
}

// startServerTmuxSession starts a server session's terminal on the server,
// by the server's rules, detached as StartTmuxSession starts a local one.
func (s *SessionsService) startServerTmuxSession(ctx context.Context, server, slug string) (multiplexer.LaunchResult, error) {
	api, err := s.serverAPI(server)
	if err != nil {
		return multiplexer.LaunchResult{}, err
	}
	sessions, err := api.ListSessions(ctx)
	if err != nil {
		return multiplexer.LaunchResult{}, serverError(err, KindUnavailable, "reading the sessions of server %q", server)
	}
	sess, ok := sessionOfTmuxName(sessions, slug)
	if !ok {
		return multiplexer.LaunchResult{}, Errorf(KindNotFound, "no session with slug %q on server %q", slug, server)
	}
	result, err := api.StartTmuxSession(ctx, sess.ID)
	switch {
	case errors.Is(err, session.ErrNotActive):
		return multiplexer.LaunchResult{}, serverError(err, KindConflict, "session %q on server %q", sess.Name, server)
	case err != nil:
		return multiplexer.LaunchResult{}, serverError(err, KindUnavailable, "starting session %q on server %q", sess.Name, server)
	}
	return result, nil
}

// sessionOfTmuxName prefers an active session, because a recycled one can keep
// the name of an active one.
func sessionOfTmuxName(sessions []session.Session, name string) (session.Session, bool) {
	named := slices.DeleteFunc(slices.Clone(sessions), func(s session.Session) bool { return sessionsvc.Target(s).Session != name })
	if len(named) == 0 {
		return session.Session{}, false
	}
	if i := slices.IndexFunc(named, func(s session.Session) bool { return s.State == session.StateActive }); i >= 0 {
		return named[i], true
	}
	return named[0], true
}

// serverError classifies a server's failure and keeps its cause in the
// message: the ssh error or the server's own refusal is the only part that
// tells the user what to fix, and Error.MarshalJSON sends the message alone.
func serverError(err error, kind Kind, format string, args ...any) error {
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, args...) + ": " + err.Error(), Err: err}
}

// errLocalOnly is the answer of an action that reads the session's checkout
// on this machine, for a session whose checkout is on a server.
func errLocalOnly(id, action string) error {
	return Errorf(KindUnavailable, "session %q is on a server; %s works only for a session on this machine", id, action)
}

// ServerLaunchOptions supplies the New Session form's choices for a server:
// its repositories and agent profiles. A server session has no workspaces.
func (s *SessionsService) ServerLaunchOptions(ctx context.Context, name string) (dispatch.SessionLaunchOptions, error) {
	api, err := s.serverAPI(name)
	if err != nil {
		return dispatch.SessionLaunchOptions{}, err
	}
	options, err := api.SessionLaunchOptions(ctx)
	if err != nil {
		return dispatch.SessionLaunchOptions{}, serverError(err, KindUnavailable, "reading the launch options of server %q", name)
	}
	opts := dispatch.SessionLaunchOptions{
		DefaultRepository:        options.DefaultRepository,
		Workspaces:               []dispatch.SessionLaunchWorkspace{},
		Agents:                   options.Agents,
		DefaultAgent:             options.DefaultAgent,
		PromptFileThresholdBytes: promptfile.ThresholdBytes,
	}
	for _, repo := range options.Repositories {
		opts.Repositories = append(opts.Repositories, dispatch.SessionLaunchRepository{Name: repo.Name, Repository: repo.Remote})
	}
	return opts, nil
}

// createOnServer creates a session on a server as a background job. The
// server applies its own rules and agent profiles. Inbox item links are not
// written, because a link names a session by its local id.
func (s *SessionsService) createOnServer(ctx context.Context, req dispatch.CreateSessionRequest, server, name, repo string) (int64, error) {
	prompt := strings.TrimSpace(req.Prompt)
	// A local launch moves a long prompt into a file, and the server cannot
	// read a file on this machine.
	if len(prompt) > promptfile.ThresholdBytes {
		return 0, Errorf(KindInvalid, "a prompt for a session on a server can be at most %d bytes", promptfile.ThresholdBytes)
	}
	api, err := s.serverAPI(server)
	if err != nil {
		return 0, err
	}
	s.logger.Info().Str("session_name", name).Str("server", server).Str("repository", repo).Str("agent", req.Agent).Msg("creating session on server")
	s.setFailedCreate(nil)
	return s.jobs.Track(ctx, "Create session", newSessionJobActionID, name, func(bg context.Context) error {
		_, err := api.CreateFromRequest(bg, sessionsvc.LaunchRequest{
			Name: name, Prompt: prompt, Agent: strings.TrimSpace(req.Agent), Repo: repo,
		})
		if err == nil {
			return nil
		}
		if errors.Is(err, session.ErrDuplicateName) {
			return fmt.Errorf("a session named %q already exists on server %q", name, server)
		}
		return s.recordFailedCreate(bg, req, err)
	}), nil
}
