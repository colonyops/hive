package daemon

import (
	"bytes"
	"context"
	"strings"
	"sync"

	"connectrpc.com/connect"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/domain/session"
	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
	"github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1/daemonv1connect"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
	"github.com/colonyops/hive/internal/hive/status"
)

type sessionServer struct {
	api    sessionapi.API
	logger zerolog.Logger

	// writeMu serializes the calls that point the session service's shared
	// stdout and stderr at a per-call progress writer. Without it two
	// concurrent creates would interleave their progress.
	writeMu sync.Mutex

	statuses func(sessions []*session.Session, roots []status.RootRepoTarget) map[string]status.TerminalStatus
}

var _ daemonv1connect.SessionServiceHandler = (*sessionServer)(nil)

func (s *sessionServer) List(ctx context.Context, _ *connect.Request[daemonv1.ListRequest]) (*connect.Response[daemonv1.ListResponse], error) {
	sessions, err := s.api.ListSessions(ctx)
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	out := make([]*daemonv1.Session, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, wire.SessionToProto(sess))
	}
	return connect.NewResponse(&daemonv1.ListResponse{Sessions: out}), nil
}

func (s *sessionServer) Get(ctx context.Context, req *connect.Request[daemonv1.GetRequest]) (*connect.Response[daemonv1.GetResponse], error) {
	sess, err := s.api.GetSession(ctx, req.Msg.GetId())
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.GetResponse{Session: wire.SessionToProto(sess)}), nil
}

func (s *sessionServer) Create(ctx context.Context, req *connect.Request[daemonv1.CreateRequest], stream *connect.ServerStream[daemonv1.CreateResponse]) error {
	if req.Msg.GetRemote() == "" {
		return wire.ToConnectError(wire.ErrRemoteRequired)
	}
	progress := newLineWriter(func(line string) error {
		return stream.Send(&daemonv1.CreateResponse{Event: &daemonv1.CreateResponse_ProgressLine{ProgressLine: line}})
	})
	opts := wire.CreateOptionsFromProto(req.Msg)
	opts.Progress = progress

	s.writeMu.Lock()
	sess, err := s.api.CreateSession(ctx, opts)
	s.writeMu.Unlock()
	progress.Flush()
	if err != nil {
		return wire.ToConnectError(err)
	}
	return stream.Send(&daemonv1.CreateResponse{Event: &daemonv1.CreateResponse_Session{Session: wire.SessionToProto(*sess)}})
}

func (s *sessionServer) CreateFromRequest(ctx context.Context, req *connect.Request[daemonv1.CreateFromRequestRequest]) (*connect.Response[daemonv1.CreateFromRequestResponse], error) {
	s.writeMu.Lock()
	sess, err := s.api.CreateFromRequest(ctx, wire.LaunchRequestFromProto(req.Msg))
	s.writeMu.Unlock()
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.CreateFromRequestResponse{Session: wire.SessionToProto(sess)}), nil
}

func (s *sessionServer) CreateWithWindows(ctx context.Context, req *connect.Request[daemonv1.CreateWithWindowsRequest]) (*connect.Response[daemonv1.CreateWithWindowsResponse], error) {
	if req.Msg.GetRemote() == "" {
		return nil, wire.ToConnectError(wire.ErrRemoteRequired)
	}
	newReq := sessionsvc.NewSessionRequest{Name: req.Msg.GetName(), Remote: req.Msg.GetRemote(), ShCmd: req.Msg.GetShCmd()}
	s.writeMu.Lock()
	sess, err := s.api.CreateSessionWithWindows(ctx, newReq, wire.WindowsFromProto(req.Msg.GetWindows()))
	s.writeMu.Unlock()
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.CreateWithWindowsResponse{Session: wire.SessionToProto(*sess)}), nil
}

func (s *sessionServer) Rename(ctx context.Context, req *connect.Request[daemonv1.RenameRequest]) (*connect.Response[daemonv1.RenameResponse], error) {
	if err := s.api.RenameSession(ctx, req.Msg.GetId(), req.Msg.GetName()); err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.RenameResponse{}), nil
}

func (s *sessionServer) SetGroup(ctx context.Context, req *connect.Request[daemonv1.SetGroupRequest]) (*connect.Response[daemonv1.SetGroupResponse], error) {
	if err := s.api.SetSessionGroup(ctx, req.Msg.GetId(), req.Msg.GetGroup()); err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.SetGroupResponse{}), nil
}

func (s *sessionServer) SetTags(ctx context.Context, req *connect.Request[daemonv1.SetTagsRequest]) (*connect.Response[daemonv1.SetTagsResponse], error) {
	sess, err := s.api.SetSessionTags(ctx, req.Msg.GetId(), req.Msg.GetTags())
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.SetTagsResponse{Session: wire.SessionToProto(sess)}), nil
}

func (s *sessionServer) EditTags(ctx context.Context, req *connect.Request[daemonv1.EditTagsRequest]) (*connect.Response[daemonv1.EditTagsResponse], error) {
	sess, err := s.api.EditSessionTags(ctx, req.Msg.GetId(), req.Msg.GetAdd(), req.Msg.GetRemove())
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.EditTagsResponse{Session: wire.SessionToProto(sess)}), nil
}

func (s *sessionServer) Delete(ctx context.Context, req *connect.Request[daemonv1.DeleteRequest]) (*connect.Response[daemonv1.DeleteResponse], error) {
	if err := s.api.DeleteSession(ctx, req.Msg.GetId()); err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.DeleteResponse{}), nil
}

func (s *sessionServer) Recycle(ctx context.Context, req *connect.Request[daemonv1.RecycleRequest], stream *connect.ServerStream[daemonv1.RecycleResponse]) error {
	progress := newLineWriter(func(line string) error {
		return stream.Send(&daemonv1.RecycleResponse{ProgressLine: line})
	})
	s.writeMu.Lock()
	err := s.api.RecycleSession(ctx, req.Msg.GetId(), progress)
	s.writeMu.Unlock()
	progress.Flush()
	return wire.ToConnectError(err)
}

func (s *sessionServer) Prune(ctx context.Context, req *connect.Request[daemonv1.PruneRequest]) (*connect.Response[daemonv1.PruneResponse], error) {
	count, err := s.api.Prune(ctx, req.Msg.GetAll())
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.PruneResponse{Count: int32(count)}), nil //nolint:gosec // a session count fits in int32
}

func (s *sessionServer) Risk(ctx context.Context, req *connect.Request[daemonv1.RiskRequest]) (*connect.Response[daemonv1.RiskResponse], error) {
	risk, err := s.api.CheckSessionRisk(ctx, req.Msg.GetId())
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.RiskResponse{
		UncommittedChanges: risk.UncommittedChanges,
		UnpushedCommits:    risk.UnpushedCommits,
	}), nil
}

func (s *sessionServer) LaunchOptions(ctx context.Context, _ *connect.Request[daemonv1.LaunchOptionsRequest]) (*connect.Response[daemonv1.LaunchOptionsResponse], error) {
	options, err := s.api.SessionLaunchOptions(ctx)
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(wire.LaunchOptionsToProto(options)), nil
}

func (s *sessionServer) Changes(ctx context.Context, req *connect.Request[daemonv1.ChangesRequest]) (*connect.Response[daemonv1.ChangesResponse], error) {
	changes, err := s.api.SessionChanges(ctx, req.Msg.GetId(), gitstatus.DiffTarget{
		Base:    gitstatus.DiffBase(req.Msg.GetBase()),
		Against: req.Msg.GetAgainst(),
		Commit:  req.Msg.GetCommit(),
	})
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(wire.ChangesToProto(changes)), nil
}

func (s *sessionServer) FileDiffs(ctx context.Context, req *connect.Request[daemonv1.FileDiffsRequest]) (*connect.Response[daemonv1.FileDiffsResponse], error) {
	diffs, err := s.api.SessionFileDiffs(ctx, req.Msg.GetId(), gitstatus.DiffTarget{
		Base:    gitstatus.DiffBase(req.Msg.GetBase()),
		Against: req.Msg.GetAgainst(),
		Commit:  req.Msg.GetCommit(),
	}, req.Msg.GetPaths())
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(wire.FileDiffsToProto(diffs)), nil
}

func (s *sessionServer) Branches(ctx context.Context, req *connect.Request[daemonv1.BranchesRequest]) (*connect.Response[daemonv1.BranchesResponse], error) {
	refs, err := s.api.SessionBranches(ctx, req.Msg.GetId())
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.BranchesResponse{Refs: refs}), nil
}

func (s *sessionServer) Statuses(ctx context.Context, req *connect.Request[daemonv1.StatusesRequest]) (*connect.Response[daemonv1.StatusesResponse], error) {
	all, err := s.api.ListSessions(ctx)
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	wanted := make(map[string]struct{}, len(req.Msg.GetSessionIds()))
	for _, id := range req.Msg.GetSessionIds() {
		wanted[id] = struct{}{}
	}
	sessions := make([]*session.Session, 0, len(wanted))
	for i := range all {
		if _, ok := wanted[all[i].ID]; ok {
			sessions = append(sessions, &all[i])
		}
	}
	statuses := s.statuses(sessions, wire.RootsFromProto(req.Msg.GetRoots()))
	return connect.NewResponse(&daemonv1.StatusesResponse{Statuses: wire.StatusesToProto(statuses)}), nil
}

// lineWriter sends each complete line written to it. The session service
// writes progress with or without a trailing newline, so Flush sends the
// rest.
type lineWriter struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	send func(string) error
	err  error
}

func newLineWriter(send func(string) error) *lineWriter {
	return &lineWriter{send: send}
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			w.buf.Reset()
			w.buf.WriteString(line)
			break
		}
		w.sendLocked(strings.TrimRight(line, "\r\n"))
	}
	return len(p), nil
}

func (w *lineWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buf.Len() > 0 {
		w.sendLocked(w.buf.String())
		w.buf.Reset()
	}
}

// sendLocked drops lines after the first failed send: the client is gone,
// and the operation itself must still finish.
func (w *lineWriter) sendLocked(line string) {
	if w.err != nil {
		return
	}
	w.err = w.send(line)
}
