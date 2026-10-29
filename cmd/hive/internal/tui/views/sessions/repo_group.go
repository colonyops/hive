package sessions

import (
	"sort"

	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/servers"
	"github.com/colonyops/hive/internal/platform/git"
)

// RepoGroup represents a repository with its associated sessions.
type RepoGroup struct {
	Remote           string            // Git remote URL (used for matching/comparison)
	Name             string            // Display name extracted from remote
	Sessions         []session.Session // Active sessions belonging to this repository
	RecycledSessions []session.Session // Recycled sessions (stored for deletion support)
	RecycledCount    int               // Number of recycled sessions (displayed as collapsed)

	// Server is set for the group of one server's sessions, with what the
	// last list found there.
	Server      string
	ServerState server.State
	ServerError string
}

// GroupSessionsByRepo groups sessions by repository, so remotes that differ
// only by transport (git.RemoteIdentity) share a group. A group's Remote is
// the remote of its first session by name. Returns groups sorted with:
// - Current repository (equivalent to localRemote) first
// - Other repositories sorted alphabetically by name
//
// Within each group, sessions are sorted by name.
func GroupSessionsByRepo(sessions []session.Session, localRemote string) []RepoGroup {
	if len(sessions) == 0 {
		return nil
	}

	groups := make(map[string]*RepoGroup)
	var serverGroups []RepoGroup
	for _, s := range sessions {
		if name := ServerOf(s); name != session.LocalServer {
			serverGroups = addToServerGroup(serverGroups, name, s)
			continue
		}
		key := git.RemoteIdentity(s.Remote)
		group, exists := groups[key]
		if !exists {
			group = &RepoGroup{Sessions: make([]session.Session, 0, 4)}
			groups[key] = group
		}
		group.Sessions = append(group.Sessions, s)
	}

	// Convert to slice, separate recycled sessions, and sort active sessions
	result := make([]RepoGroup, 0, len(groups))
	for _, group := range groups {
		sortSessions(group.Sessions)
		group.Remote = group.Sessions[0].Remote
		group.Name = extractGroupName(group.Remote)

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
		result = append(result, *group)
	}

	sortRepoGroups(result, localRemote)

	return append(result, serverGroups...)
}

// ServerOf names the server a session is on: the server part of its
// "<server>:<id>" id, or session.LocalServer.
func ServerOf(s session.Session) string {
	ref, err := session.ParseRef(s.ID, session.LocalServer)
	if err != nil {
		return session.LocalServer
	}
	return ref.Server
}

// gitKey keys a session's git status: its path on this machine, or its
// qualified id for a server's session, whose path may also exist here.
func gitKey(s session.Session) string {
	if ServerOf(s) != session.LocalServer {
		return s.ID
	}
	return s.Path
}

func addToServerGroup(groups []RepoGroup, name string, s session.Session) []RepoGroup {
	for i := range groups {
		if groups[i].Server == name {
			if s.State == session.StateRecycled {
				groups[i].RecycledSessions = append(groups[i].RecycledSessions, s)
				groups[i].RecycledCount++
			} else {
				groups[i].Sessions = append(groups[i].Sessions, s)
				sortSessions(groups[i].Sessions)
			}
			return groups
		}
	}
	return addToServerGroup(append(groups, RepoGroup{Name: name, Server: name}), name, s)
}

// WithServerStates puts a group for every server after the local groups, in
// config order, with what the last list found there. A server with no
// sessions, such as one that is offline, still gets its group.
func WithServerStates(groups []RepoGroup, states []servers.ListState) []RepoGroup {
	if len(states) == 0 {
		return groups
	}
	byServer := map[string]RepoGroup{}
	out := make([]RepoGroup, 0, len(groups)+len(states))
	for _, g := range groups {
		if g.Server != "" {
			byServer[g.Server] = g
			continue
		}
		out = append(out, g)
	}
	for _, st := range states {
		g, ok := byServer[st.Name]
		if !ok {
			g = RepoGroup{Name: st.Name, Server: st.Name}
		}
		g.ServerState = st.State
		if st.Err != nil {
			g.ServerError = st.Err.Error()
		}
		out = append(out, g)
	}
	return out
}

// extractGroupName returns the display name for a repository group.
func extractGroupName(remote string) string {
	if remote == "" {
		return "(no remote)"
	}
	return git.ExtractRepoName(remote)
}

// sortSessions sorts sessions alphabetically by name.
func sortSessions(sessions []session.Session) {
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].Name != sessions[j].Name {
			return sessions[i].Name < sessions[j].Name
		}
		return sessions[i].ID < sessions[j].ID
	})
}

// sortRepoGroups sorts repository groups with local repo first, then alphabetically.
func sortRepoGroups(groups []RepoGroup, localRemote string) {
	sort.Slice(groups, func(i, j int) bool {
		iLocal := git.EquivalentRemote(groups[i].Remote, localRemote)
		jLocal := git.EquivalentRemote(groups[j].Remote, localRemote)

		if iLocal != jLocal {
			return iLocal
		}
		return groups[i].Name < groups[j].Name
	})
}

// GroupSessionsByTag groups sessions by their user-assigned group metadata.
// Sessions without a group are placed in an "(ungrouped)" group.
// Returns groups sorted alphabetically by name.
//
// Within each group, sessions are sorted by name.
func GroupSessionsByTag(sessions []session.Session) []RepoGroup {
	if len(sessions) == 0 {
		return nil
	}

	const ungrouped = "(ungrouped)"

	groups := make(map[string]*RepoGroup)
	for _, s := range sessions {
		groupName := s.Group()
		if groupName == "" {
			groupName = ungrouped
		}

		group, exists := groups[groupName]
		if !exists {
			group = &RepoGroup{
				Name:     groupName,
				Sessions: make([]session.Session, 0, 4),
			}
			groups[groupName] = group
		}
		group.Sessions = append(group.Sessions, s)
	}

	// Convert to slice, separate recycled sessions, and sort active sessions
	result := make([]RepoGroup, 0, len(groups))
	for _, group := range groups {
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

	// Sort groups alphabetically, with "(ungrouped)" last
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == ungrouped {
			return false
		}
		if result[j].Name == ungrouped {
			return true
		}
		return result[i].Name < result[j].Name
	})

	return result
}
