package actions

import (
	"sort"
	"strings"
	"sync"
)

// ActionStore holds the actions loaded from an actions.yml file, reloading
// on demand (Reload). On a broken reload (parse/validation failure) it
// retains the last-good action set — the same last-good-on-failure posture
// feed.Store and flow.FlowStore take — so a half-edited actions.yml degrades
// (edits don't take effect) rather than blanking every action out from
// under a running flow or the detail pane.
//
// Thread-safe: every method takes the same mutex.
type ActionStore struct {
	path string

	mu      sync.Mutex
	loaded  bool
	actions map[string]Action
	err     error // last Reload error, if any; retained actions are last-good
}

// NewActionStore returns a store over path (typically
// desktop.ActionsPath()). Nothing is read from disk until the first
// List/Get/Reload call.
func NewActionStore(path string) *ActionStore {
	return &ActionStore{path: path}
}

// List returns every loaded action, sorted by id.
func (s *ActionStore) List() []Action {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()

	out := make([]Action, 0, len(s.actions))
	for _, a := range s.actions {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ViewsFor returns the frontend view for every action that applies to kind,
// sorted by id. An action with no applies_to restriction applies to every
// kind. Kind comparisons are case-insensitive because GitHub items use
// "PR"/"Issue" while actions.yml examples conventionally use lowercase.
func (s *ActionStore) ViewsFor(kind string) []View {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()

	out := make([]View, 0, len(s.actions))
	for _, action := range s.actions {
		if actionAppliesTo(action, kind) {
			out = append(out, action.View())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AppliesTo reports whether action is available for kind.
func AppliesTo(action Action, kind string) bool {
	return actionAppliesTo(action, kind)
}

func actionAppliesTo(action Action, kind string) bool {
	if len(action.AppliesTo) == 0 {
		return true
	}
	for _, allowed := range action.AppliesTo {
		if strings.EqualFold(allowed, kind) {
			return true
		}
	}
	return false
}

// Get returns one loaded action by id.
func (s *ActionStore) Get(id string) (Action, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()

	a, ok := s.actions[id]
	return a, ok
}

// Err returns the error from the most recent Reload (or initial lazy load),
// or nil if the actions file loaded cleanly (including "file does not
// exist", which is not itself an error — see LoadActions).
func (s *ActionStore) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	return s.err
}

// Reload re-reads the actions file, retaining the last-good action set when
// the new content fails to parse or validate.
func (s *ActionStore) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reloadLocked()
}

func (s *ActionStore) reloadLocked() error {
	loaded, err := LoadActions(s.path)
	s.loaded = true
	if err != nil {
		s.err = err
		return err
	}

	byID := make(map[string]Action, len(loaded))
	for _, a := range loaded {
		byID[a.ID] = a
	}
	s.actions = byID
	s.err = nil
	return nil
}

func (s *ActionStore) ensureLoadedLocked() {
	if s.loaded {
		return
	}
	// First-use lazy load: a broken actions file surfaces through Err()
	// (and List/Get simply return nothing) rather than panicking callers
	// that haven't called Reload explicitly.
	_ = s.reloadLocked()
}
