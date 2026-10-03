package sessions

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/internal/hive/gitstatus"
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
