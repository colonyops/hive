package flow

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Seed is the graph a newly created flow starts with, plus the canvas
// positions for it. Create takes one from its caller rather than building it:
// a starter graph names source connectors, and this package is
// connector-neutral by design.
//
// The zero Seed is an empty graph, which is a valid flow — a workspace is the
// thing that exists before any credential does.
type Seed struct {
	Nodes  []Node
	Wires  []Wire
	Layout Layout
}

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
// emits "flows:updated" so the frontend knows to re-fetch and reconcile its
// running graph snapshots. This store only ever swaps its own in-memory
// snapshot; it does not touch a running graph.
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
	order  map[string]int // flow id -> rail position, from SetOrder
}

// NewFlowStore returns a store over dir (typically desktop.FlowsDir()),
// resolving action-node references through refs. Nothing is read from disk
// until the first List/Get/Save/Statuses call.
func NewFlowStore(dir string, refs Refs) *FlowStore {
	return &FlowStore{dir: dir, refs: refs}
}

// List returns every successfully loaded flow in rail order — see SetOrder.
// Flows whose file failed to load are omitted — see Statuses for the full
// picture, including broken files.
func (s *FlowStore) List() []Flow {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()

	out := make([]Flow, 0, len(s.flows))
	for _, f := range s.flows {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return s.lessLocked(out[i].ID, out[j].ID) })
	return out
}

// SetOrder sets the rail order: the ids given sort first, in the order given,
// and every other flow sorts alphabetically after them. An id naming no flow
// is ignored rather than rejected, so deleting a profile never invalidates the
// setting that named it, and an id the caller repeats keeps its first
// position.
//
// Order is process state, not flow state: it comes from settings.yaml, which
// this package does not read, and a reload of the flows directory leaves it
// alone.
func (s *FlowStore) SetOrder(ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	order := make(map[string]int, len(ids))
	for _, id := range ids {
		if _, dup := order[id]; !dup {
			order[id] = len(order)
		}
	}
	s.order = order
}

// lessLocked is the one comparator every listing sorts by. Caller holds s.mu.
func (s *FlowStore) lessLocked(a, b string) bool {
	ra, rb := s.rankLocked(a), s.rankLocked(b)
	if ra != rb {
		return ra < rb
	}
	return a < b
}

// rankLocked is id's configured rail position; an id the order omits ranks
// behind every id it names. Caller holds s.mu.
func (s *FlowStore) rankLocked(id string) int {
	if rank, ok := s.order[id]; ok {
		return rank
	}
	return math.MaxInt
}

// Get returns one loaded flow by id.
func (s *FlowStore) Get(id string) (Flow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()

	f, ok := s.flows[id]
	return f, ok
}

// Exists reports whether a flow file backs id, parsing or not. It is a weaker
// question than Get's: a file that fails to validate has no Flow to return but
// is very much there, and deleting or renaming it is how that gets fixed.
func (s *FlowStore) Exists(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()

	if _, ok := s.flows[id]; ok {
		return true
	}
	for filename := range s.errs {
		if flowIDFromFilename(filename) == id {
			return true
		}
	}
	return false
}

// ParseDocument parses a flow document that is not on disk, resolving its
// references the same way a loaded file's are. Nothing is written and the
// store's own snapshot is untouched — this is how an unsaved edit is checked,
// or executed by a dry run, without being deployed first.
func (s *FlowStore) ParseDocument(id string, data []byte) (Flow, []string, error) {
	return ParseDocument(id, data, s.refs)
}

// Statuses returns one FlowStatus per flow file in the directory — valid
// and invalid alike — in rail order, for a listing UI that must surface
// broken flows too, not just the ones that loaded. A broken file sorts by the
// same rule a working one does: ordering the rail by one comparator and its
// error entries by another would interleave them unpredictably.
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
	sort.Slice(out, func(i, j int) bool { return s.lessLocked(out[i].ID, out[j].ID) })
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
	s.ensureLoadedLocked()

	if _, err := validateFlow(&f, s.refs); err != nil {
		return fmt.Errorf("flow %q: %w", f.ID, err)
	}

	// The avatar reference is owned by SetImage, not the graph editor, which
	// does not round-trip it — take whatever the loaded flow already declares
	// so a graph save never drops the image.
	if cur, ok := s.flows[f.ID]; ok {
		f.Image = cur.Image
	}

	path := filepath.Join(s.dir, f.ID+".yaml")
	if err := SaveFlow(path, f); err != nil {
		return err
	}
	return s.reloadLocked()
}

// SetImage sets a flow's avatar reference — the content hash of its stored
// image, or "" to clear it — without touching its graph or other fields. Like
// SetEnabled it reads the file directly rather than the possibly-stale cached
// snapshot, so a concurrent external graph edit is not reverted.
func (s *FlowStore) SetImage(id, hash string) (Flow, error) {
	if !validSlug(id) {
		return Flow{}, fmt.Errorf("flow: id %q is not a valid slug", id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, id+".yaml")
	f, _, err := LoadFlow(path, s.refs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Flow{}, fmt.Errorf("flow %q not found", id)
		}
		return Flow{}, err
	}
	f.Image = hash
	if err := SaveFlow(path, f); err != nil {
		return Flow{}, err
	}
	if err := s.reloadLocked(); err != nil {
		return Flow{}, err
	}
	return s.flows[id], nil
}

// Create writes a new flow (a new "profile") named name: it slugifies name to
// a flow id unique among existing flows, writes seed's graph plus its layout,
// reloads, and returns the loaded flow. A profile is a flow, so this is how
// the app's "New profile" affordance is backed.
//
// An empty seed writes an empty graph, which is what a workspace created
// before any account is connected gets: a source node carries a credential
// ref, so there is no unconfigured source node to stand in for one.
func (s *FlowStore) Create(name string, seed Seed) (Flow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()

	id := s.uniqueIDLocked(slugify(name))
	f := Flow{ID: id, Name: name, Enabled: true, Resurface: ResurfacePolicyStateChanges, Nodes: seed.Nodes, Wires: seed.Wires}
	if _, err := validateFlow(&f, s.refs); err != nil {
		return Flow{}, fmt.Errorf("flow %q: %w", id, err)
	}

	if err := SaveFlow(filepath.Join(s.dir, id+".yaml"), f); err != nil {
		return Flow{}, err
	}
	if err := SaveUI(s.uiPath(id), seed.Layout); err != nil {
		return Flow{}, err
	}
	if err := s.reloadLocked(); err != nil {
		return Flow{}, err
	}
	return s.flows[id], nil
}

// Rename updates a flow's display name without changing its stable id or any
// graph content.
func (s *FlowStore) Rename(id, name string) (Flow, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Flow{}, fmt.Errorf("flow: name cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()

	f, ok := s.flows[id]
	if !ok {
		return Flow{}, fmt.Errorf("flow %q not found", id)
	}
	f.Name = name
	if _, err := validateFlow(&f, s.refs); err != nil {
		return Flow{}, fmt.Errorf("flow %q: %w", f.ID, err)
	}
	if err := SaveFlow(filepath.Join(s.dir, f.ID+".yaml"), f); err != nil {
		return Flow{}, err
	}
	if err := s.reloadLocked(); err != nil {
		return Flow{}, err
	}
	return s.flows[id], nil
}

// SetEnabled updates whether a flow participates in polling and runtime
// execution without changing its graph or display name.
func (s *FlowStore) SetEnabled(id string, enabled bool) (Flow, error) {
	if !validSlug(id) {
		return Flow{}, fmt.Errorf("flow: id %q is not a valid slug", id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Read the file directly instead of mutating the cached snapshot. The flow
	// watcher is debounced, so an external graph edit may already be on disk but
	// not yet reflected in s.flows; writing that stale snapshot would revert it.
	path := filepath.Join(s.dir, id+".yaml")
	f, _, err := LoadFlow(path, s.refs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Flow{}, fmt.Errorf("flow %q not found", id)
		}
		return Flow{}, err
	}
	f.Enabled = enabled
	if err := SaveFlow(path, f); err != nil {
		return Flow{}, err
	}
	if err := s.reloadLocked(); err != nil {
		return Flow{}, err
	}
	return s.flows[id], nil
}

// Delete removes a flow (a "profile") and its sibling layout file, then
// reloads. A missing flow file is not an error — the end state (gone) is the
// same either way.
func (s *FlowStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(filepath.Join(s.dir, id+".yaml")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("flow: delete %q: %w", id, err)
	}
	_ = os.Remove(s.uiPath(id))      // layout is cosmetic; its absence is fine
	_ = os.Remove(s.sidebarPath(id)) // sidebar layout is cosmetic too
	return s.reloadLocked()
}

// uniqueIDLocked returns base, or base-2/base-3/... if a flow with that id
// already exists. Caller holds s.mu.
func (s *FlowStore) uniqueIDLocked(base string) string {
	taken := func(id string) bool {
		if _, ok := s.flows[id]; ok {
			return true
		}
		_, ok := s.errs[id+".yaml"]
		return ok
	}
	id := base
	for i := 2; taken(id); i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	return id
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

// GetSidebar returns id's sidebar layout (feed folders + ordering) — see
// LoadSidebar for missing/broken-file semantics (an empty layout, never an
// error).
func (s *FlowStore) GetSidebar(id string) SidebarLayout {
	return LoadSidebar(s.sidebarPath(id))
}

// SaveSidebar persists id's sidebar layout.
func (s *FlowStore) SaveSidebar(id string, layout SidebarLayout) error {
	return SaveSidebar(s.sidebarPath(id), layout)
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
	// file) or an empty List/Get (nothing loaded), matching the store's
	// last-good-on-failure posture rather than panicking callers that
	// haven't called Reload explicitly.
	_ = s.reloadLocked()
}

func (s *FlowStore) uiPath(id string) string {
	return filepath.Join(s.dir, id+".ui.yaml")
}

func (s *FlowStore) sidebarPath(id string) string {
	return filepath.Join(s.dir, id+".sidebar.yaml")
}
