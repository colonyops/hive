// Package sessionapi is the session view the hive TUI, the CLI, and Hive
// Desktop share. Local runs it on an in-process engine; the daemon client
// runs it over RPC.
package sessionapi

import (
	"context"
	"errors"
	"io"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/status"
)

// ErrRiskCheckTimedOut is a risk check that the server began and did not finish,
// as with a slow repository. A caller can offer to go on without the check.
var ErrRiskCheckTimedOut = errors.New("the risk check timed out")

// API creates every session in the background, whatever opts.Background
// says: the implementation may run in a daemon with no terminal. A caller
// that wants the session in front attaches or switches to the one returned.
type API interface {
	ListSessions(ctx context.Context) ([]session.Session, error)
	GetSession(ctx context.Context, id string) (session.Session, error)
	// CreateSession writes progress to opts.Progress when set. Callers
	// resolve an empty opts.Remote and pass an absolute opts.Source, because
	// the implementation may run in another process.
	CreateSession(ctx context.Context, opts sessionsvc.CreateOptions) (*session.Session, error)
	CreateFromRequest(ctx context.Context, req sessionsvc.LaunchRequest) (session.Session, error)
	CreateSessionWithWindows(ctx context.Context, req sessionsvc.NewSessionRequest, windows []multiplexer.WindowSpec) (*session.Session, error)
	RenameSession(ctx context.Context, id, name string) error
	SetSessionGroup(ctx context.Context, id, group string) error
	SetSessionTags(ctx context.Context, id string, tags []string) (session.Session, error)
	EditSessionTags(ctx context.Context, id string, add, remove []string) (session.Session, error)
	DeleteSession(ctx context.Context, id string) error
	RecycleSession(ctx context.Context, id string, w io.Writer) error
	Prune(ctx context.Context, all bool) (int, error)
	CheckSessionRisk(ctx context.Context, id string) (sessionsvc.Risk, error)
	SessionLaunchOptions(ctx context.Context) (sessionsvc.LaunchOptions, error)
	SessionChanges(ctx context.Context, id string, target gitstatus.DiffTarget) (gitstatus.Changes, error)
	SessionFileDiffs(ctx context.Context, id string, target gitstatus.DiffTarget, paths []string) ([]gitstatus.FileDiff, error)
	SessionBranches(ctx context.Context, id string) ([]string, error)
	// GitStatuses reads the checkouts of the sessions ids name on the machine
	// that holds them, keyed by id. An id with no session is absent.
	GitStatuses(ctx context.Context, ids []string) (map[string]gitstatus.Status, error)

	// StatusAvailable must not block: the TUI calls it inside Update. A false
	// answer can turn true later (the daemon client answers false until its
	// first Hello returns), so callers ask again instead of caching it.
	StatusAvailable() bool
	FetchBatch(ctx context.Context, sessions []*session.Session, roots []status.RootRepoTarget) map[string]status.TerminalStatus
}
