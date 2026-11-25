package engineops

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/tagset"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/sessions"
	"github.com/colonyops/hive/internal/platform/git"
)

// sessionAPI is the session API of the named server; empty or
// session.LocalServer is this machine.
func (h *Handlers) sessionAPI(server string) (sessionapi.API, error) {
	if server == "" {
		server = session.LocalServer
	}
	fleet := h.currentFleet()
	if server == session.LocalServer {
		if fleet == nil {
			return sessionapi.NewLocal(h.engine), nil
		}
		if local := fleet.Local(); local != nil {
			return local, nil
		}
		return nil, ops.Errorf(ops.KindUnavailable, "this process has no session API for this machine")
	}
	if fleet != nil {
		if srv, ok := fleet.Server(server); ok {
			return srv.Client(), nil
		}
	}
	return nil, ops.Errorf(ops.KindNotFound, "unknown server %q: it is not under servers: in config.yaml", server)
}

// target reads a session id, "<server>:<id>" or a bare id on server, and
// finds the session API that holds it.
func (h *Handlers) target(sessionID, server string) (session.Ref, sessionapi.API, error) {
	if sessionID == "" {
		return session.Ref{}, nil, ops.Errorf(ops.KindInvalid, "sessionId is required")
	}
	ref, err := session.ParseRef(sessionID, cmpOr(server, session.LocalServer))
	if err != nil {
		return session.Ref{}, nil, ops.Errorf(ops.KindInvalid, "%v", err)
	}
	api, err := h.sessionAPI(ref.Server)
	if err != nil {
		return session.Ref{}, nil, err
	}
	return ref, api, nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func sessionOut(server string, s session.Session) sessions.SessionOut {
	tags := s.Tags
	if tags == nil {
		tags = []string{}
	}
	return sessions.SessionOut{
		Server:        server,
		ID:            s.ID,
		Name:          s.Name,
		Slug:          s.Slug,
		Repo:          git.ExtractRepoName(s.Remote),
		Remote:        s.Remote,
		Path:          s.Path,
		Inbox:         s.InboxTopic(),
		State:         string(s.State),
		Group:         s.GetMeta(session.MetaGroup),
		CloneStrategy: s.CloneStrategy,
		Tags:          tags,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

type serverListing struct {
	server   string
	sessions []session.Session
	err      error
}

// serverListTimeout bounds one server's part of a list that reads every
// server, so a stuck server costs the list seconds and not the client's connect
// timeout. A fresh ssh connection is about 14 round trips, so a shorter bound
// reports a healthy server on a slow link as offline.
const serverListTimeout = 10 * time.Second

// listEachServer reads the sessions of the named server, or of this machine
// and every server at once.
func (h *Handlers) listEachServer(ctx context.Context, server string) ([]serverListing, error) {
	fleet := h.currentFleet()
	if server != "" || fleet == nil || len(fleet.Servers()) == 0 {
		name := cmpOr(server, session.LocalServer)
		api, err := h.sessionAPI(name)
		if err != nil {
			return nil, err
		}
		list, err := api.ListSessions(ctx)
		return []serverListing{{server: name, sessions: list, err: err}}, nil
	}
	remotes := fleet.Servers()
	out := make([]serverListing, len(remotes)+1)
	var wg sync.WaitGroup
	wg.Go(func() {
		api, err := h.sessionAPI(session.LocalServer)
		if err != nil {
			out[0] = serverListing{server: session.LocalServer, err: err}
			return
		}
		list, err := api.ListSessions(ctx)
		out[0] = serverListing{server: session.LocalServer, sessions: list, err: err}
	})
	for i, srv := range remotes {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(ctx, serverListTimeout)
			defer cancel()
			list, err := srv.Client().ListSessions(ctx)
			out[i+1] = serverListing{server: srv.Config().Name, sessions: list, err: err}
		})
	}
	wg.Wait()
	return out, nil
}

func (h *Handlers) SessionList(ctx context.Context, in sessions.SessionListIn) (sessions.SessionListOut, error) {
	listings, err := h.listEachServer(ctx, in.Server)
	if err != nil {
		return sessions.SessionListOut{}, err
	}
	fanOut := len(listings) > 1
	out := sessions.SessionListOut{Sessions: []sessions.SessionOut{}}
	for _, listing := range listings {
		if listing.err != nil {
			// A server that does not answer must not hide the sessions that
			// did, so only this machine, or the one server named, fails the
			// call.
			if !fanOut || listing.server == session.LocalServer {
				return sessions.SessionListOut{}, opError(listing.err, "list sessions")
			}
			h.log.Debug().Err(listing.err).Str("server", listing.server).Msg("server did not list its sessions")
			out.Offline = append(out.Offline, sessions.ServerFailure{Server: listing.server, Error: listing.err.Error()})
			continue
		}
		var mine []sessions.SessionOut
		for _, s := range listing.sessions {
			// A corrupted session is reported whatever the tag filter, so
			// a filtered list still says it needs a prune.
			if in.State == "" && s.State == session.StateCorrupted {
				out.Corrupted = append(out.Corrupted, h.listedSession(ctx, listing.server, s))
				continue
			}
			if !s.HasAllTags(in.Tags) || (in.State != "" && string(s.State) != in.State) {
				continue
			}
			mine = append(mine, h.listedSession(ctx, listing.server, s))
		}
		slices.SortStableFunc(mine, func(a, b sessions.SessionOut) int { return strings.Compare(a.Repo, b.Repo) })
		out.Sessions = append(out.Sessions, mine...)
	}
	return out, nil
}

// listedSession is a session as a list shows it: the inbox and its unread
// count for a session on this machine only, because messages sent from here
// never reach a server's hive.db.
func (h *Handlers) listedSession(ctx context.Context, server string, s session.Session) sessions.SessionOut {
	out := sessionOut(server, s)
	if server != session.LocalServer {
		out.Inbox = ""
		return out
	}
	unread := 0
	msgs, err := h.engine.Messages().GetUnread(ctx, s.ID, s.InboxTopic())
	if err != nil {
		h.log.Warn().Err(err).Str("session", s.ID).Msg("read unread messages")
	} else {
		unread = len(msgs)
	}
	out.Unread = &unread
	return out
}

func (h *Handlers) SessionGet(ctx context.Context, in sessions.SessionGetIn) (sessions.SessionOut, error) {
	ref, api, err := h.target(in.SessionID, in.Server)
	if err != nil {
		return sessions.SessionOut{}, err
	}
	s, err := api.GetSession(ctx, ref.ID)
	if err != nil {
		return sessions.SessionOut{}, opError(err, "get session "+ref.String())
	}
	return sessionOut(ref.Server, s), nil
}

func (h *Handlers) SessionUpdate(ctx context.Context, in sessions.SessionUpdateIn) (sessions.SessionOut, error) {
	ref, api, err := h.target(in.SessionID, in.Server)
	if err != nil {
		return sessions.SessionOut{}, err
	}
	// Checked before any write, so a bad tag leaves the session untouched.
	add, err := tagset.Normalize(in.AddTag)
	if err != nil {
		return sessions.SessionOut{}, ops.Errorf(ops.KindInvalid, "%v", err)
	}
	if in.Name != "" {
		if err := api.RenameSession(ctx, ref.ID, in.Name); err != nil {
			return sessions.SessionOut{}, opError(err, "rename session")
		}
	}
	if in.Group != "" || in.ClearGroup {
		if err := api.SetSessionGroup(ctx, ref.ID, in.Group); err != nil {
			return sessions.SessionOut{}, opError(err, "set session group")
		}
	}
	switch {
	case in.ClearTags:
		tags := slices.DeleteFunc(add, func(tag string) bool {
			return slices.ContainsFunc(in.RemoveTag, func(remove string) bool {
				return strings.EqualFold(strings.TrimSpace(remove), tag)
			})
		})
		if _, err := api.SetSessionTags(ctx, ref.ID, tags); err != nil {
			return sessions.SessionOut{}, opError(err, "set session tags")
		}
	case len(add) > 0 || len(in.RemoveTag) > 0:
		if _, err := api.EditSessionTags(ctx, ref.ID, add, in.RemoveTag); err != nil {
			return sessions.SessionOut{}, opError(err, "edit session tags")
		}
	}
	s, err := api.GetSession(ctx, ref.ID)
	if err != nil {
		return sessions.SessionOut{}, opError(err, "get session "+ref.String())
	}
	return sessionOut(ref.Server, s), nil
}

func (h *Handlers) SessionTagsUpdate(ctx context.Context, in sessions.SessionTagsUpdateIn) (sessions.SessionOut, error) {
	ref, api, err := h.target(in.SessionID, in.Server)
	if err != nil {
		return sessions.SessionOut{}, err
	}
	s, err := api.EditSessionTags(ctx, ref.ID, in.Add, in.Remove)
	if err != nil {
		return sessions.SessionOut{}, opError(err, "edit session tags")
	}
	return sessionOut(ref.Server, s), nil
}

func (h *Handlers) SessionDelete(ctx context.Context, in sessions.SessionDeleteIn) (sessions.SessionDeletedOut, error) {
	ref, api, err := h.target(in.SessionID, in.Server)
	if err != nil {
		return sessions.SessionDeletedOut{}, err
	}
	if !in.Force {
		if err := checkRisk(ctx, api, ref, "delete"); err != nil {
			return sessions.SessionDeletedOut{}, err
		}
	}
	if err := api.DeleteSession(ctx, ref.ID); err != nil {
		return sessions.SessionDeletedOut{}, opError(err, "delete session")
	}
	return sessions.SessionDeletedOut{Server: ref.Server, ID: ref.ID, Deleted: true}, nil
}

func (h *Handlers) SessionRecycle(ctx context.Context, in sessions.SessionRecycleIn) (sessions.SessionRecycleOut, error) {
	ref, api, err := h.target(in.SessionID, in.Server)
	if err != nil {
		return sessions.SessionRecycleOut{}, err
	}
	if !in.Force {
		if err := checkRisk(ctx, api, ref, "recycle"); err != nil {
			return sessions.SessionRecycleOut{}, err
		}
	}
	if err := api.RecycleSession(ctx, ref.ID, h.progress); err != nil {
		return sessions.SessionRecycleOut{}, opError(err, "recycle session")
	}
	// A worktree session is deleted on recycle, so the record may be gone.
	s, err := api.GetSession(ctx, ref.ID)
	switch {
	case errors.Is(err, session.ErrNotFound):
		return sessions.SessionRecycleOut{Deleted: &sessions.SessionDeletedOut{Server: ref.Server, ID: ref.ID, Deleted: true}}, nil
	case err != nil:
		// The recycle is done; failing now would make a caller retry it.
		h.log.Warn().Err(err).Str("session", ref.String()).Msg("read session after recycle")
		return sessions.SessionRecycleOut{Session: &sessions.SessionOut{
			Server: ref.Server, ID: ref.ID, State: string(session.StateRecycled), Tags: []string{},
		}}, nil
	}
	out := sessionOut(ref.Server, s)
	return sessions.SessionRecycleOut{Session: &out}, nil
}

// checkRisk refuses an action that would lose uncommitted or unpushed work.
func checkRisk(ctx context.Context, api sessionapi.API, ref session.Ref, action string) error {
	risk, err := api.CheckSessionRisk(ctx, ref.ID)
	if err != nil {
		if errors.Is(err, sessionapi.ErrRiskCheckTimedOut) {
			return ops.Errorf(ops.KindUnavailable, "check %s for unsaved work: %v; use --force to %s without this check", ref, err, action)
		}
		return opError(err, "check "+ref.String()+" for unsaved work")
	}
	if !risk.HasRisk() {
		return nil
	}
	var reasons []string
	if risk.UncommittedChanges {
		reasons = append(reasons, "uncommitted changes")
	}
	if risk.UnpushedCommits {
		reasons = append(reasons, "unpushed commits")
	}
	return ops.Errorf(ops.KindConflict, "session %s has %s; use --force to %s anyway", ref, strings.Join(reasons, " and "), action)
}

func (h *Handlers) SessionCreate(ctx context.Context, in sessions.SessionCreateIn) (sessions.SessionOut, error) {
	if strings.TrimSpace(in.Name) == "" {
		return sessions.SessionOut{}, ops.Errorf(ops.KindInvalid, "name is required")
	}
	server := cmpOr(in.Server, session.LocalServer)
	onServer := server != session.LocalServer
	api, err := h.sessionAPI(server)
	if err != nil {
		return sessions.SessionOut{}, err
	}
	if err := h.checkAgent(ctx, api, server, in.Agent); err != nil {
		return sessions.SessionOut{}, err
	}
	// A server cannot read a path on this machine, so a create there gets no
	// source and its copy: rules copy nothing.
	if in.Source != "" && onServer {
		return sessions.SessionOut{}, ops.Errorf(ops.KindInvalid, "--source names a directory on this machine, which server %s cannot read", server)
	}
	if in.Source != "" && !filepath.IsAbs(in.Source) {
		return sessions.SessionOut{}, ops.Errorf(ops.KindInvalid, "source %q is not an absolute path", in.Source)
	}
	dir, err := h.callerDir(ctx, in.Caller)
	if err != nil {
		return sessions.SessionOut{}, err
	}
	opts := sessionsvc.CreateOptions{
		Name: in.Name, Remote: in.Remote, Source: in.Source, Background: true,
		CloneStrategy: in.CloneStrategy, AgentKey: in.Agent, Tags: in.Tags, Progress: h.progress,
	}
	if opts.Source == "" && !onServer {
		opts.Source = dir
	}
	if opts.Remote == "" {
		from := cmpOr(dir, opts.Source)
		if from == "" {
			return sessions.SessionOut{}, ops.Errorf(ops.KindInvalid, "remote is required when the caller names no directory")
		}
		remote, err := h.engine.Sessions().DetectRemote(ctx, from)
		if err != nil {
			return sessions.SessionOut{}, ops.Errorf(ops.KindInvalid, "detect remote: %v", err)
		}
		opts.Remote = remote
		if onServer && isLocalPath(remote) {
			return sessions.SessionOut{}, ops.Errorf(ops.KindInvalid,
				"this checkout's remote %q is a path on this machine, which the server cannot clone; pass --remote with a URL the server can reach", remote)
		}
	}
	created, err := api.CreateSession(ctx, opts)
	if err != nil {
		return sessions.SessionOut{}, opError(err, "create session")
	}
	return sessionOut(server, *created), nil
}

// callerDir is the directory a create reads its defaults from: a path
// caller, else the caller session's checkout.
func (h *Handlers) callerDir(ctx context.Context, value string) (string, error) {
	if filepath.IsAbs(value) {
		return value, nil
	}
	c, err := h.resolve(ctx, value)
	if err != nil {
		return "", err
	}
	if c.Session != nil {
		return c.Session.Path, nil
	}
	return c.Dir, nil
}

// checkAgent refuses an agent profile that the machine which spawns the
// session does not have.
func (h *Handlers) checkAgent(ctx context.Context, api sessionapi.API, server, agent string) error {
	if agent == "" {
		return nil
	}
	if server == session.LocalServer {
		if _, ok := h.engine.Config().Agents.Profiles[agent]; !ok {
			return ops.Errorf(ops.KindInvalid, "unknown agent %q", agent)
		}
		return nil
	}
	options, err := api.SessionLaunchOptions(ctx)
	if err != nil {
		return opError(err, "read the agent profiles of server "+server)
	}
	if !slices.Contains(options.Agents, agent) {
		return ops.Errorf(ops.KindInvalid, "server %s has no agent %q; its agents are %s", server, agent, strings.Join(options.Agents, ", "))
	}
	return nil
}

// isLocalPath reports whether a git remote names a path on this machine
// instead of a URL another machine can clone.
func isLocalPath(remote string) bool {
	return strings.HasPrefix(remote, "/") || strings.HasPrefix(remote, ".") ||
		strings.HasPrefix(remote, "~") || strings.HasPrefix(remote, "file://")
}

func (h *Handlers) SessionStart(ctx context.Context, in sessions.SessionStartIn) (sessions.SessionStartOut, error) {
	ref, api, err := h.target(in.SessionID, in.Server)
	if err != nil {
		return sessions.SessionStartOut{}, err
	}
	result, err := api.StartTmuxSession(ctx, ref.ID)
	if err != nil {
		return sessions.SessionStartOut{}, opError(err, fmt.Sprintf("start session %s", ref))
	}
	return sessions.SessionStartOut{Server: ref.Server, ID: ref.ID, Created: result.Created, Completed: result.Completed}, nil
}
