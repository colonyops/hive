package servers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
	"github.com/colonyops/hive/internal/hive/status"
)

const (
	// ServerListTimeout bounds one server's part of a list, so a server that
	// stops answering costs a few seconds instead of ssh's own timeout.
	ServerListTimeout = 5 * time.Second
	// serverProbeTimeout bounds a list of an offline server. No caller waits for
	// it, so it can outlast ssh's connect timeout and report ssh's reason, and a
	// slow link can still come back.
	serverProbeTimeout = 30 * time.Second
)

// Router is one session API over this machine and every server. It lists
// the sessions of all of them, gives a server's session the id
// "<server>:<id>" (session.Ref), and sends a call that names an id to the
// server that id names. Creates, prune, and launch options act on this
// machine; a caller that creates on a server uses that server's API.
type Router struct {
	fleet *Fleet

	mu      sync.Mutex
	listed  map[string]ListState
	last    map[string][]session.Session
	probing map[string]bool
}

// ListState is what the last list found on one server.
type ListState struct {
	Name  string
	State server.State
	Err   error
}

var _ sessionapi.API = (*Router)(nil)

func NewRouter(fleet *Fleet) *Router {
	return &Router{fleet: fleet, listed: map[string]ListState{}, last: map[string][]session.Session{}, probing: map[string]bool{}}
}

// States returns what the last ListSessions found on each server, in config
// order. A server the router has not listed yet is connecting.
func (r *Router) States() []ListState {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ListState, 0, len(r.fleet.Servers()))
	for _, srv := range r.fleet.Servers() {
		name := srv.Config().Name
		st, ok := r.listed[name]
		if !ok {
			st = ListState{Name: name, State: server.StateConnecting}
		}
		out = append(out, st)
	}
	return out
}

// ListSessions fails only when the local list fails. A server that does not
// answer keeps the sessions of its last good list, so a short outage does not
// move the user's selection to another row; States reports why. The list does
// not wait for a server that is already offline: it asks that server in the
// background, and the next list has the answer.
func (r *Router) ListSessions(ctx context.Context) ([]session.Session, error) {
	remotes := r.fleet.Servers()
	var wg sync.WaitGroup
	for _, srv := range remotes {
		if r.offline(srv.Config().Name) {
			r.probe(srv)
			continue
		}
		wg.Go(func() { r.listServer(ctx, srv, ServerListTimeout) })
	}
	local, err := r.fleet.Local().ListSessions(ctx)
	wg.Wait()
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, srv := range remotes {
		local = append(local, r.last[srv.Config().Name]...)
	}
	return local, nil
}

func (r *Router) listServer(ctx context.Context, srv *Server, timeout time.Duration) {
	name := srv.Config().Name
	listCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	sessions, err := srv.Client().ListSessions(listCtx)
	st := ListState{Name: name, State: server.StateConnected}
	if err != nil {
		st.State = server.StateOffline
		if _, ok := errors.AsType[*wire.IncompatibleError](err); ok {
			st.State = server.StateIncompatible
		}
		st.Err = err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listed[name] = st
	if err != nil {
		return
	}
	list := make([]session.Session, 0, len(sessions))
	for _, sess := range sessions {
		list = append(list, qualify(name, sess))
	}
	r.last[name] = list
}

func (r *Router) probe(srv *Server) {
	name := srv.Config().Name
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.probing[name] {
		return
	}
	r.probing[name] = true
	go func() {
		r.listServer(context.Background(), srv, serverProbeTimeout)
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.probing, name)
	}()
}

func (r *Router) GetSession(ctx context.Context, id string) (session.Session, error) {
	ref, api, err := r.route(id)
	if err != nil {
		return session.Session{}, err
	}
	sess, err := api.GetSession(ctx, ref.ID)
	return qualify(ref.Server, sess), err
}

func (r *Router) CreateSession(ctx context.Context, opts sessionsvc.CreateOptions) (*session.Session, error) {
	return r.fleet.Local().CreateSession(ctx, opts)
}

func (r *Router) CreateFromRequest(ctx context.Context, req sessionsvc.LaunchRequest) (session.Session, error) {
	return r.fleet.Local().CreateFromRequest(ctx, req)
}

func (r *Router) CreateSessionWithWindows(ctx context.Context, req sessionsvc.NewSessionRequest, windows []multiplexer.WindowSpec) (*session.Session, error) {
	return r.fleet.Local().CreateSessionWithWindows(ctx, req, windows)
}

func (r *Router) RenameSession(ctx context.Context, id, name string) error {
	ref, api, err := r.route(id)
	if err != nil {
		return err
	}
	return api.RenameSession(ctx, ref.ID, name)
}

func (r *Router) SetSessionGroup(ctx context.Context, id, group string) error {
	ref, api, err := r.route(id)
	if err != nil {
		return err
	}
	return api.SetSessionGroup(ctx, ref.ID, group)
}

func (r *Router) SetSessionTags(ctx context.Context, id string, tags []string) (session.Session, error) {
	ref, api, err := r.route(id)
	if err != nil {
		return session.Session{}, err
	}
	sess, err := api.SetSessionTags(ctx, ref.ID, tags)
	return qualify(ref.Server, sess), err
}

func (r *Router) EditSessionTags(ctx context.Context, id string, add, remove []string) (session.Session, error) {
	ref, api, err := r.route(id)
	if err != nil {
		return session.Session{}, err
	}
	sess, err := api.EditSessionTags(ctx, ref.ID, add, remove)
	return qualify(ref.Server, sess), err
}

func (r *Router) DeleteSession(ctx context.Context, id string) error {
	ref, api, err := r.route(id)
	if err != nil {
		return err
	}
	return api.DeleteSession(ctx, ref.ID)
}

func (r *Router) RecycleSession(ctx context.Context, id string, w io.Writer) error {
	ref, api, err := r.route(id)
	if err != nil {
		return err
	}
	return api.RecycleSession(ctx, ref.ID, w)
}

func (r *Router) Prune(ctx context.Context, all bool) (int, error) {
	return r.fleet.Local().Prune(ctx, all)
}

func (r *Router) CheckSessionRisk(ctx context.Context, id string) (sessionsvc.Risk, error) {
	ref, api, err := r.route(id)
	if err != nil {
		return sessionsvc.Risk{}, err
	}
	return api.CheckSessionRisk(ctx, ref.ID)
}

func (r *Router) SessionLaunchOptions(ctx context.Context) (sessionsvc.LaunchOptions, error) {
	return r.fleet.Local().SessionLaunchOptions(ctx)
}

func (r *Router) StartTmuxSession(ctx context.Context, id string) (multiplexer.LaunchResult, error) {
	ref, api, err := r.route(id)
	if err != nil {
		return multiplexer.LaunchResult{}, err
	}
	return api.StartTmuxSession(ctx, ref.ID)
}

// GitStatuses reads each server's checkouts on that server, at the same time.
// A server that does not answer leaves its sessions out.
func (r *Router) GitStatuses(ctx context.Context, ids []string) (map[string]gitstatus.Status, error) {
	byServer, err := r.split(ids)
	if err != nil {
		return nil, err
	}
	out := map[string]gitstatus.Status{}
	var mu sync.Mutex
	var localErr error
	var wg sync.WaitGroup
	for name, bare := range byServer {
		if r.offline(name) {
			continue
		}
		wg.Go(func() {
			api, err := r.api(name)
			if err != nil {
				return
			}
			statuses, err := api.GitStatuses(ctx, bare)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if name == session.LocalServer {
					localErr = err
				}
				return
			}
			for id, st := range statuses {
				out[session.Ref{Server: name, ID: id}.String()] = st
			}
		})
	}
	wg.Wait()
	return out, localErr
}

// StatusAvailable is true when this machine or any server can report
// terminal status.
func (r *Router) StatusAvailable() bool {
	available := r.fleet.Local().StatusAvailable()
	for _, srv := range r.fleet.Servers() {
		if r.connected(srv.Config().Name) && srv.Client().StatusAvailable() {
			available = true
		}
	}
	return available
}

// connected reports whether the last list reached the server. Status polls
// skip a server that did not answer, so an offline server costs one ssh
// attempt per list instead of one per poll.
func (r *Router) connected(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listed[name].State == server.StateConnected
}

// offline reports whether the last list failed. A server not listed yet is
// not offline.
func (r *Router) offline(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.listed[name]
	return ok && st.State != server.StateConnected
}

// FetchBatch asks each server about its own sessions, at the same time. Roots
// are directories on this machine, so only the local call gets them.
func (r *Router) FetchBatch(ctx context.Context, sessions []*session.Session, roots []status.RootRepoTarget) map[string]status.TerminalStatus {
	byServer := map[string][]*session.Session{}
	for _, sess := range sessions {
		ref, err := session.ParseRef(sess.ID, session.LocalServer)
		if err != nil {
			continue
		}
		bare := *sess
		bare.ID = ref.ID
		byServer[ref.Server] = append(byServer[ref.Server], &bare)
	}
	out := map[string]status.TerminalStatus{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	fetch := func(name string, api sessionapi.API, list []*session.Session, roots []status.RootRepoTarget) {
		if !api.StatusAvailable() {
			return
		}
		statuses := api.FetchBatch(ctx, list, roots)
		mu.Lock()
		defer mu.Unlock()
		for key, st := range statuses {
			if name != session.LocalServer {
				key = session.Ref{Server: name, ID: key}.String()
			}
			out[key] = st
		}
	}
	wg.Go(func() { fetch(session.LocalServer, r.fleet.Local(), byServer[session.LocalServer], roots) })
	for name, list := range byServer {
		if name == session.LocalServer {
			continue
		}
		srv, ok := r.fleet.Server(name)
		if !ok || !r.connected(name) {
			continue
		}
		wg.Go(func() { fetch(name, srv.Client(), list, nil) })
	}
	wg.Wait()
	return out
}

func (r *Router) api(name string) (sessionapi.API, error) {
	if name == session.LocalServer {
		return r.fleet.Local(), nil
	}
	srv, ok := r.fleet.Server(name)
	if !ok {
		return nil, fmt.Errorf("unknown server %q: it is not under servers: in config.yaml", name)
	}
	return srv.Client(), nil
}

func (r *Router) route(id string) (session.Ref, sessionapi.API, error) {
	ref, err := session.ParseRef(id, session.LocalServer)
	if err != nil {
		return session.Ref{}, nil, err
	}
	api, err := r.api(ref.Server)
	return ref, api, err
}

func (r *Router) split(ids []string) (map[string][]string, error) {
	out := map[string][]string{}
	for _, id := range ids {
		ref, err := session.ParseRef(id, session.LocalServer)
		if err != nil {
			return nil, err
		}
		out[ref.Server] = append(out[ref.Server], ref.ID)
	}
	return out, nil
}

func qualify(server string, sess session.Session) session.Session {
	if server != session.LocalServer && sess.ID != "" {
		sess.ID = session.Ref{Server: server, ID: sess.ID}.String()
	}
	return sess
}
