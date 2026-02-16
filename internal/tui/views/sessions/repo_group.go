package sessions

import (
	"sort"

	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/session"
)

// RepoGroup represents a repository with its associated sessions.
type RepoGroup struct {
	Remote           string            // Git remote URL (used for matching/comparison)
	Name             string            // Display name extracted from remote
	Sessions         []session.Session // Active sessions belonging to this repository
	RecycledSessions []session.Session // Recycled sessions (stored for deletion support)
	RecycledCount    int               // Number of recycled sessions (displayed as collapsed)
}

// GroupSessionsByRepo groups sessions by their repository remote URL.
// Sessions are grouped by their Remote field. Returns groups sorted with:
// - Current repository (matching localRemote) first
// - Other repositories sorted alphabetically by name
//
// Within each group, sessions are sorted by name.
func GroupSessionsByRepo(sessions []session.Session, localRemote string) []RepoGroup {
	if len(sessions) == 0 {
		return nil
	}

	// Group sessions by remote URL
	groups := make(map[string]*RepoGroup)
	for _, s := range sessions {
		remote := s.Remote
		if remote == "" {
			remote = "(no remote)"
		}

		group, exists := groups[remote]
		if !exists {
			group = &RepoGroup{
				Remote:   remote,
				Name:     extractGroupName(remote),
				Sessions: make([]session.Session, 0, 4),
			}
			groups[remote] = group
		}
		group.Sessions = append(group.Sessions, s)
	}

	// Convert to slice, separate recycled sessions, and sort active sessions
	result := make([]RepoGroup, 0, len(groups))
	for _, group := range groups {
		// Separate active and recycled sessions
		activeSessions := make([]session.Session, 0, len(group.Sessions))
		recycledSessions := make([]session.Session, 0)
		for _, s := range group.Sessions {
			if s.State == session.StateRecycled {
				recycledSessions = append(recycledSessions, s)
			} else {
				activeSessions = append(activeSessions, s)
			}
		}
		group.Sessions = activeSessions
		group.RecycledSessions = recycledSessions
		group.RecycledCount = len(recycledSessions)
		sortSessions(group.Sessions)
		result = append(result, *group)
	}

	// Sort groups: local repo first, then alphabetically by name
	sortRepoGroups(result, localRemote)

	return result
}

// extractGroupName returns the display name for a repository group.
func extractGroupName(remote string) string {
	if remote == "" || remote == "(no remote)" {
		return "(no remote)"
	}
	return git.ExtractRepoName(remote)
}

// sortSessions sorts sessions alphabetically by name.
// Note: Recycled sessions are now separated and counted, not included in this slice.
func sortSessions(sessions []session.Session) {
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Name < sessions[j].Name
	})
}

// sortRepoGroups sorts repository groups with local repo first, then alphabetically.
func sortRepoGroups(groups []RepoGroup, localRemote string) {
	sort.Slice(groups, func(i, j int) bool {
		iLocal := groups[i].Remote == localRemote
		jLocal := groups[j].Remote == localRemote

		// Local repo always comes first
		if iLocal != jLocal {
			return iLocal
		}

		// Otherwise sort alphabetically by name
		return groups[i].Name < groups[j].Name
	})
}
