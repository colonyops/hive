package gitstatus

import (
	"context"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/platform/git"
)

const maxChangedFiles = 1000

// ENUM(branch, uncommitted)
type DiffBase string

// Changes is what differs in a session's checkout against a base. Base is the
// base the read used, which is uncommitted whenever BranchAvailable is false:
// on the default branch, or when the default branch or merge base can't be
// resolved, as in a shallow clone.
type Changes struct {
	Resolved        bool
	Branch          string
	DefaultBranch   string
	Base            DiffBase
	BranchAvailable bool
	Files           []git.ChangedFile
	Truncated       bool
}

// ReadChanges answers the zero Changes for a session with no checkout, as
// ReadSession does: there is nothing to read, which is not a failure. Files
// stops at maxChangedFiles.
func (s *Service) ReadChanges(ctx context.Context, sess session.Session, base DiffBase) (Changes, error) {
	if sess.State != session.StateActive || sess.Path == "" {
		return Changes{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	branch, err := s.git.Branch(ctx, sess.Path)
	if err != nil {
		return Changes{}, err
	}
	defaultBranch, mergeBase := s.branchBase(ctx, sess.Path, branch)

	changes := Changes{
		Resolved:        true,
		Branch:          branch,
		DefaultBranch:   defaultBranch,
		Base:            DiffBaseUncommitted,
		BranchAvailable: mergeBase != "",
	}
	ref := "HEAD"
	if changes.BranchAvailable && base != DiffBaseUncommitted {
		changes.Base = DiffBaseBranch
		ref = mergeBase
	}

	files, err := s.git.ChangedFiles(ctx, sess.Path, ref)
	if err != nil {
		return Changes{}, err
	}
	if len(files) > maxChangedFiles {
		files = files[:maxChangedFiles]
		changes.Truncated = true
	}
	changes.Files = files
	return changes, nil
}

// branchBase returns an empty merge base when the branch has nothing to compare
// against, which callers show as the uncommitted view rather than as a failure.
func (s *Service) branchBase(ctx context.Context, dir, branch string) (defaultBranch, mergeBase string) {
	defaultBranch, err := s.git.DefaultBranch(ctx, dir)
	if err != nil {
		s.logger.Debug().Err(err).Str("path", dir).Msg("git default branch lookup failed")
		return "", ""
	}
	if defaultBranch == branch {
		return defaultBranch, ""
	}
	mergeBase, err = s.git.MergeBase(ctx, dir, defaultBranch)
	if err != nil {
		s.logger.Debug().Err(err).Str("path", dir).Msg("git merge base lookup failed")
		return defaultBranch, ""
	}
	return defaultBranch, mergeBase
}
