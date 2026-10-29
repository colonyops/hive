package sessions

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/internal/hive/gitstatus"
	"github.com/colonyops/hive/internal/hive/sessionapi"
)

// GitStatus holds the git status information for a session.
type GitStatus struct {
	Branch     string
	Additions  int
	Deletions  int
	HasChanges bool
	IsLoading  bool
	Error      error
}

// GitStatusBatchCompleteMsg is sent when all git status fetches complete.
type GitStatusBatchCompleteMsg struct {
	Results map[string]GitStatus
}

// FetchGitStatusBatch returns a command that reads git status for every path.
// The tree does not show unpushed commits, so it skips that read.
func FetchGitStatusBatch(svc *gitstatus.Service, paths []string) tea.Cmd {
	if len(paths) == 0 {
		return nil
	}

	return func() tea.Msg {
		read := svc.ReadBatch(context.Background(), paths, gitstatus.Options{})
		results := make(map[string]GitStatus, len(read))
		for path, status := range read {
			results[path] = GitStatus{
				Branch:     status.Branch,
				Additions:  status.Additions,
				Deletions:  status.Deletions,
				HasChanges: status.Dirty,
				Error:      status.Err,
			}
		}
		return GitStatusBatchCompleteMsg{Results: results}
	}
}

// FetchServerGitStatusBatch reads the git status of server sessions on their
// servers, keyed by qualified id like gitKey.
func FetchServerGitStatusBatch(api sessionapi.API, ids []string) tea.Cmd {
	if len(ids) == 0 {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), serverGitStatusTimeout)
		defer cancel()
		read, err := api.GitStatuses(ctx, ids)
		results := make(map[string]GitStatus, len(ids))
		for _, id := range ids {
			status, ok := read[id]
			if !ok {
				results[id] = GitStatus{Error: errors.Join(errServerGitStatus, err)}
				continue
			}
			results[id] = GitStatus{
				Branch:     status.Branch,
				Additions:  status.Additions,
				Deletions:  status.Deletions,
				HasChanges: status.Dirty,
				Error:      status.Err,
			}
		}
		return GitStatusBatchCompleteMsg{Results: results}
	}
}

const serverGitStatusTimeout = 10 * time.Second

var errServerGitStatus = errors.New("the server did not report git status")
