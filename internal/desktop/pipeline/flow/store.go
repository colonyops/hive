package flow

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// FlowStatus is one flows/*.yaml file's load outcome, keyed by the flow id
// (the filename stem — see LoadFlow). Valid flows carry Flow and any soft
// Warnings; invalid files carry Err instead and a zero Flow, so a listing
// UI can show a broken flow file (and why) rather than have it silently
// vanish from the list.
type FlowStatus struct {
	ID       string
	Flow     Flow
	Valid    bool
	Err      error
	Warnings []string
}

// FlowStore holds the flows loaded from a flows/*.yaml directory, reloading
// on demand (Reload) or from a fsnotify FlowsWatcher. It is the backend
// half of Deploy: a flows-dir change reaches here via Reload, and the app
// emits "flows:updated" so the frontend knows to re-fetch — the frontend
// then performs the actual drain-then-swap of the running graph on receipt
// (Phase 6). This store only ever swaps its own in-memory snapshot; it does
// not touch a running graph.
//
// Thread-safe: every method takes the same mutex.
type FlowStore struct {
	dir  string
	refs Refs

	mu     sync.Mutex
	loaded bool
	flows  map[string]Flow
	errs   map[string]error // filename -> load error, for broken files
	warns  map[string][]string
}

// NewFlowStore returns a store over dir (typically desktop.FlowsDir()),
// resolving cross-file references (sources, feeds, actions) through refs.
// Nothing is read from disk until the first List/Get/Save/Statuses call.
func NewFlowStore(dir string, refs Refs) *FlowStore {
	return &FlowStore{dir: dir, refs: refs}
}

// List returns every successfully loaded flow, sorted by id. Flows whose
// file failed to load are omitted — see Statuses for the full picture,
// including broken files.
func (s *FlowStore) List() []Flow {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()

	out := make([]Flow, 0, len(s.flows))
	for _, f := range s.flows {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns one loaded flow by id.
func (s *FlowStore) Get(id string) (Flow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()

	f, ok := s.flows[id]
	return f, ok
}

// Statuses returns one FlowStatus per flow file in the directory — valid
// and invalid alike — sorted by id, for a listing UI that must surface
// broken flows too, not just the ones that loaded.
func (s *FlowStore) Statuses() []FlowStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()

	out := make([]FlowStatus, 0, len(s.flows)+len(s.errs))
	for _, f := range s.flows {
		out = append(out, FlowStatus{ID: f.ID, Flow: f, Valid: true, Warnings: s.warns[f.ID]})
	}
	for filename, err := range s.errs {
		out = append(out, FlowStatus{ID: flowIDFromFilename(filepath.Base(filename)), Err: err})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Save re-validates f (the same checks LoadFlow runs, via the store's
// Refs) and, only if that passes, writes it with SaveFlow and reloads. An
// invalid flow is rejected before anything is written, so the file on disk
// — and the store's in-memory state — is untouched: the last-good flow
// keeps serving.
func (s *FlowStore) Save(f Flow) error {
	if !validSlug(f.ID) {
		return fmt.Errorf("flow: id %q is not a valid slug", f.ID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := validateFlow(&f, s.refs); err != nil {
		return fmt.Errorf("flow %q: %w", f.ID, err)
	}

	path := filepath.Join(s.dir, f.ID+".yaml")
	if err := SaveFlow(path, f); err != nil {
		return err
	}
	return s.reloadLocked()
}

// GetLayout returns id's node layout — see LoadUI for missing/broken-file
// semantics (an empty Layout, never an error).
func (s *FlowStore) GetLayout(id string) Layout {
	return LoadUI(s.uiPath(id))
}

// SaveLayout persists id's node layout.
func (s *FlowStore) SaveLayout(id string, layout Layout) error {
	return SaveUI(s.uiPath(id), layout)
}

// Reload re-reads every flow file in the directory. It only returns an
// error when the directory itself can't be created/read (rare — the flows
// dir is created on demand); a broken individual flow file is never a
// Reload error, it just shows up in Statuses/Errors.
func (s *FlowStore) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reloadLocked()
}

func (s *FlowStore) reloadLocked() error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("flow: create flows dir: %w", err)
	}
	flows, perFileErrors, warnings := LoadFlows(s.dir, s.refs)

	byID := make(map[string]Flow, len(flows))
	for _, f := range flows {
		byID[f.ID] = f
	}
	s.flows = byID
	s.errs = perFileErrors
	s.warns = warnings
	s.loaded = true
	return nil
}

func (s *FlowStore) ensureLoadedLocked() {
	if s.loaded {
		return
	}
	// First-use lazy load: errors surface through Statuses (a broken flow
	// file) or an empty List/Get (nothing loaded), matching feed.Store's
	// last-good-on-failure posture rather than panicking callers that
	// haven't called Reload explicitly.
	_ = s.reloadLocked()
}

func (s *FlowStore) uiPath(id string) string {
	return filepath.Join(s.dir, id+".ui.yaml")
}
