package wire

import (
	"errors"

	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
	"github.com/colonyops/hive/internal/hive/gitstatus"
)

func GitStatusesToProto(statuses map[string]gitstatus.Status) map[string]*daemonv1.GitStatus {
	out := make(map[string]*daemonv1.GitStatus, len(statuses))
	for id, s := range statuses {
		out[id] = &daemonv1.GitStatus{
			Path:          s.Path,
			Branch:        s.Branch,
			Dirty:         s.Dirty,
			Unpushed:      s.Unpushed,
			Additions:     int32(s.Additions), //nolint:gosec // line counts of one diff
			Deletions:     int32(s.Deletions), //nolint:gosec // line counts of one diff
			Host:          s.Host,
			Owner:         s.Owner,
			Repo:          s.Repo,
			Resolved:      s.Resolved,
			Error:         errorText(s.Err),
			UnpushedError: errorText(s.UnpushedErr),
		}
	}
	return out
}

// GitStatusesFromProto returns the failed reads as plain errors with the
// daemon's text: their types do not cross the wire.
func GitStatusesFromProto(statuses map[string]*daemonv1.GitStatus) map[string]gitstatus.Status {
	out := make(map[string]gitstatus.Status, len(statuses))
	for id, s := range statuses {
		out[id] = gitstatus.Status{
			Path:        s.GetPath(),
			Branch:      s.GetBranch(),
			Dirty:       s.GetDirty(),
			Unpushed:    s.GetUnpushed(),
			Additions:   int(s.GetAdditions()),
			Deletions:   int(s.GetDeletions()),
			Host:        s.GetHost(),
			Owner:       s.GetOwner(),
			Repo:        s.GetRepo(),
			Resolved:    s.GetResolved(),
			Err:         textError(s.GetError()),
			UnpushedErr: textError(s.GetUnpushedError()),
		}
	}
	return out
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func textError(text string) error {
	if text == "" {
		return nil
	}
	return errors.New(text)
}
