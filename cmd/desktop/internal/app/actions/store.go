package actions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ActionUsage describes references that make deleting an action unsafe.
type ActionUsage struct {
	FlowIDs        []string
	ActiveCommands int64
}

// ActionUsageChecker is deliberately narrow so actions does not depend on
// flow or the pipeline database.
type ActionUsageChecker interface {
	Usage(ctx context.Context, actionID string) (ActionUsage, error)
}

// ActionStore owns actions.yml — both of the lists in it, the actions and the
// launchers — and retains its last-good snapshot if a disk reload or mutation
// candidate is invalid. All mutations re-read disk while holding this lock.
//
// Each sequence's order is that list's presentation order, so the snapshot
// keeps the parsed slices as-is and indexes them separately for lookup by id;
// nothing re-sorts on the way out.
type ActionStore struct {
	path      string
	mu        sync.Mutex
	loaded    bool
	actions   []Action
	index     map[string]Action
	launchers []Launcher
	err       error
	usage     ActionUsageChecker
}

func NewActionStore(path string) *ActionStore { return &ActionStore{path: path} }

// usageChecker returns a stable checker reference without keeping the action
// lock while it may query FlowStore. Delete deliberately treats its result as
// a preflight: a flow or queue item may begin referencing an action after the
// check and before the locked disk mutation.
func (s *ActionStore) usageChecker() ActionUsageChecker {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usage
}

func (s *ActionStore) SetUsageChecker(checker ActionUsageChecker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage = checker
}

func (s *ActionStore) List() []Action {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	return append(make([]Action, 0, len(s.actions)), s.actions...)
}

func AppliesTo(action Action, kind string) bool { return actionAppliesTo(action, kind) }
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

// Launchers returns the configured launchers in file order.
func (s *ActionStore) Launchers() []Launcher {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	return append(make([]Launcher, 0, len(s.launchers)), s.launchers...)
}

func (s *ActionStore) Launcher(id string) (Launcher, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	for _, l := range s.launchers {
		if l.ID == id {
			return l, true
		}
	}
	return Launcher{}, false
}

func (s *ActionStore) Get(id string) (Action, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	a, ok := s.index[id]
	return a, ok
}

func (s *ActionStore) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	return s.err
}
func (s *ActionStore) Reload() error { s.mu.Lock(); defer s.mu.Unlock(); return s.reloadLocked() }
func (s *ActionStore) reloadLocked() error {
	catalog, err := LoadCatalog(s.path)
	s.loaded = true
	if err != nil {
		s.err = err
		return err
	}
	s.actions = catalog.Actions
	s.index = byID(catalog.Actions)
	s.launchers = catalog.Launchers
	s.err = nil
	return nil
}

func (s *ActionStore) ensureLoadedLocked() {
	if !s.loaded {
		_ = s.reloadLocked()
	}
}

func (s *ActionStore) ListEditable() EditableCatalog {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	out := make([]EditableAction, 0, len(s.actions))
	catalog := EditableCatalog{}
	for _, a := range s.actions {
		e, err := editableFromAction(a)
		if err != nil {
			// A registered type with no editable-catalog branch is a
			// registry/editable.go mismatch, not a bad hand edit: surface it
			// the same way a disk parse error is surfaced rather than
			// silently omitting the action from the catalog.
			catalog.Error = err.Error()
			continue
		}
		out = append(out, e)
	}
	catalog.Actions = out
	catalog.Launchers = append(make([]Launcher, 0, len(s.launchers)), s.launchers...)
	if s.err != nil {
		catalog.Error = s.err.Error()
	}
	return catalog
}

func (s *ActionStore) Create(e EditableAction) (EditableAction, error) {
	a, err := actionFromEditable(e)
	if err != nil {
		return EditableAction{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateLocked("create", a.ID, a)
}

func (s *ActionStore) Update(ctx context.Context, id string, e EditableAction) (EditableAction, error) {
	if id != e.ID {
		return EditableAction{}, fmt.Errorf("action id is immutable")
	}
	a, err := actionFromEditable(e)
	if err != nil {
		return EditableAction{}, err
	}

	// As with Delete, do not call Usage while holding s.mu: FlowStore Save/Create
	// can resolve actions while holding the flow lock. This is intentionally a
	// preflight, so a flow may start referencing the action before the locked
	// disk mutation. Only a headless-to-interactive transition is unsafe for a
	// loaded flow; active-command checks remain specific to Delete.
	current, ok := s.Get(id)
	if !ok {
		return EditableAction{}, fmt.Errorf("action %q not found", id)
	}
	if current.HeadlessCapable() && !a.HeadlessCapable() {
		if checker := s.usageChecker(); checker != nil {
			usage, err := checker.Usage(ctx, id)
			if err != nil {
				return EditableAction{}, fmt.Errorf("check action %q usage: %w", id, err)
			}
			if len(usage.FlowIDs) > 0 {
				return EditableAction{}, fmt.Errorf("action %q is referenced by flow(s): %s; it cannot become interactive-only", id, strings.Join(usage.FlowIDs, ", "))
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateLocked("update", id, a)
}

func (s *ActionStore) Delete(ctx context.Context, id string) error {
	// Do not call Usage while holding s.mu: FlowStore Save/Create validate
	// action references while holding their flow lock, so doing so would invert
	// the action -> flow lock order. This preflight intentionally has the race
	// boundary documented on usageChecker.
	if checker := s.usageChecker(); checker != nil {
		usage, err := checker.Usage(ctx, id)
		if err != nil {
			return fmt.Errorf("check action %q usage: %w", id, err)
		}
		if len(usage.FlowIDs) > 0 || usage.ActiveCommands > 0 {
			reasons := make([]string, 0, 2)
			if len(usage.FlowIDs) > 0 {
				reasons = append(reasons, "flows: "+strings.Join(usage.FlowIDs, ", "))
			}
			if usage.ActiveCommands > 0 {
				reasons = append(reasons, fmt.Sprintf("%d nonterminal output command(s) (pending or running)", usage.ActiveCommands))
			}
			return fmt.Errorf("action %q is in use (%s)", id, strings.Join(reasons, "; "))
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	doc, list, err := s.latestDocumentLocked("actions")
	if err != nil {
		return err
	}
	i := findNodeByID(list, id)
	if i < 0 {
		return fmt.Errorf("action %q not found", id)
	}
	list.Content = append(list.Content[:i], list.Content[i+1:]...)
	return s.writeDocumentLocked(doc)
}

// Reorder rewrites the actions.yml sequence so it reads in ids order, which is
// the order every caller of List/ListEditable then sees. ids must name each
// action currently on disk exactly once: a reorder computed from a catalog that
// a hand edit has since changed is rejected rather than allowed to drop or
// duplicate an entry. An order that already matches disk writes nothing.
func (s *ActionStore) Reorder(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, list, err := s.latestDocumentLocked("actions")
	if err != nil {
		return err
	}
	if len(ids) != len(list.Content) {
		return fmt.Errorf("reorder actions: %d ids for %d actions on disk; the catalog changed", len(ids), len(list.Content))
	}
	ordered := make([]*yaml.Node, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return fmt.Errorf("reorder actions: action %q listed twice", id)
		}
		seen[id] = true
		i := findNodeByID(list, id)
		if i < 0 {
			return fmt.Errorf("reorder actions: action %q not found", id)
		}
		ordered = append(ordered, list.Content[i])
	}
	if sameNodes(list.Content, ordered) {
		return nil
	}
	list.Content = ordered
	return s.writeDocumentLocked(doc)
}

// CreateLauncher and UpdateLauncher write one launcher into the `launchers:`
// list, through the same read-latest → edit-node → validate → atomic-write path
// action CRUD takes, so a hand edit that arrived since is never clobbered.
func (s *ActionStore) CreateLauncher(l Launcher) (Launcher, error) {
	return s.mutateLauncher("create", l.ID, l)
}

func (s *ActionStore) UpdateLauncher(id string, l Launcher) (Launcher, error) {
	if id != l.ID {
		return Launcher{}, fmt.Errorf("launcher id is immutable")
	}
	return s.mutateLauncher("update", id, l)
}

// DeleteLauncher needs no usage preflight, unlike an action: nothing references
// a launcher but a keybinding, and a binding for an id that is gone simply
// stops resolving.
func (s *ActionStore) DeleteLauncher(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, list, err := s.latestDocumentLocked("launchers")
	if err != nil {
		return err
	}
	i := findNodeByID(list, id)
	if i < 0 {
		return fmt.Errorf("launcher %q not found", id)
	}
	list.Content = append(list.Content[:i], list.Content[i+1:]...)
	return s.writeDocumentLocked(doc)
}

func (s *ActionStore) mutateLauncher(mode, id string, l Launcher) (Launcher, error) {
	if err := l.Validate(); err != nil {
		return Launcher{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, list, err := s.latestDocumentLocked("launchers")
	if err != nil {
		return Launcher{}, err
	}
	i := findNodeByID(list, id)
	switch {
	case mode == "create" && i >= 0:
		return Launcher{}, fmt.Errorf("launcher %q already exists", id)
	case mode == "create":
		list.Content = append(list.Content, launcherNode(l))
	case i < 0:
		return Launcher{}, fmt.Errorf("launcher %q not found", id)
	default:
		list.Content[i] = launcherNode(l)
	}
	if err := s.writeDocumentLocked(doc); err != nil {
		return Launcher{}, err
	}
	return l, nil
}

func launcherNode(l Launcher) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	add := func(k, v string) { n.Content = append(n.Content, scalar(k), scalar(v)) }
	add("id", l.ID)
	add("label", l.Label)
	add("command", l.Command)
	if l.Cwd != "" {
		add("cwd", l.Cwd)
	}
	if l.Icon != "" {
		add("icon", l.Icon)
	}
	return n
}

func sameNodes(a, b []*yaml.Node) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *ActionStore) mutateLocked(mode, id string, a Action) (EditableAction, error) {
	doc, list, err := s.latestDocumentLocked("actions")
	if err != nil {
		return EditableAction{}, err
	}
	i := findNodeByID(list, id)
	if mode == "create" {
		if i >= 0 {
			return EditableAction{}, fmt.Errorf("action %q already exists", id)
		}
		node, err := actionNode(a)
		if err != nil {
			return EditableAction{}, err
		}
		list.Content = append(list.Content, node)
	} else {
		if i < 0 {
			return EditableAction{}, fmt.Errorf("action %q not found", id)
		}
		node, err := actionNode(a)
		if err != nil {
			return EditableAction{}, err
		}
		list.Content[i] = node
	}
	if err := s.writeDocumentLocked(doc); err != nil {
		return EditableAction{}, err
	}
	return editableFromAction(a)
}

// latestDocumentLocked rejects invalid latest disk bytes before altering disk
// or memory, and answers the document's writable sequence under key ("actions"
// or "launchers"). Empty present files are valid and become a new v1 document.
func (s *ActionStore) latestDocumentLocked(key string) (*yaml.Node, *yaml.Node, error) {
	data, err := os.ReadFile(s.path)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("read actions %q: %w", s.path, err)
	}
	if err == nil {
		if _, err := parseCatalog(data); err != nil {
			return nil, nil, fmt.Errorf("actions file changed to invalid content: %w", err)
		}
	}
	if os.IsNotExist(err) || len(strings.TrimSpace(string(data))) == 0 {
		doc := newDocument()
		return doc, normalizeSequence(doc.Content[0], key), nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("parse actions document: %w", err)
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("actions: valid document has invalid root")
	}
	return &doc, normalizeSequence(root, key), nil
}

// normalizeSequence makes an optional/null/absent top-level list field
// writable. LoadCatalog accepts all three shapes as an empty list, so CRUD must
// too. It changes only that field, retaining all unrelated mapping keys and
// comments — which is what lets a hand-authored file keep its comments through
// an edit made in the app.
func normalizeSequence(root *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != key {
			continue
		}
		if root.Content[i+1].Kind == yaml.SequenceNode {
			return root.Content[i+1]
		}
		old := root.Content[i+1]
		list := &yaml.Node{
			Kind:        yaml.SequenceNode,
			Tag:         "!!seq",
			HeadComment: old.HeadComment,
			LineComment: old.LineComment,
			FootComment: old.FootComment,
		}
		root.Content[i+1] = list
		return list
	}
	list := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	root.Content = append(root.Content, scalar(key), list)
	return list
}

func newDocument() *yaml.Node {
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	root.Content = []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: "version"}, {Kind: yaml.ScalarNode, Tag: "!!int", Value: "1"}}
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
}

func findNodeByID(list *yaml.Node, id string) int {
	for i, n := range list.Content {
		for j := 0; j+1 < len(n.Content); j += 2 {
			if n.Content[j].Value == "id" && n.Content[j+1].Value == id {
				return i
			}
		}
	}
	return -1
}

func (s *ActionStore) writeDocumentLocked(doc *yaml.Node) error {
	data, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode actions: %w", err)
	}
	if _, err := parseCatalog(data); err != nil {
		return fmt.Errorf("validate action change: %w", err)
	}
	if err := atomicWrite(s.path, data); err != nil {
		if _, ok := errors.AsType[*installedWriteError](err); ok {
			// Rename already made the valid candidate visible. Reload despite a
			// durability-sync error so this store never keeps serving stale data.
			if reloadErr := s.reloadLocked(); reloadErr != nil {
				return fmt.Errorf("%w; reload installed actions: %w", err, reloadErr)
			}
		}
		return err
	}
	return s.reloadLocked()
}

func byID(list []Action) map[string]Action {
	out := make(map[string]Action, len(list))
	for _, a := range list {
		out[a.ID] = a
	}
	return out
}

func scalar(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }

func actionNode(a Action) (*yaml.Node, error) {
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	add := func(k, v string) { n.Content = append(n.Content, scalar(k), scalar(v)) }
	add("id", a.ID)
	add("label", a.Label)
	add("type", a.Type)
	if a.ShowInDetail {
		n.Content = append(n.Content, scalar("show_in_detail"), &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
	}
	if len(a.Targets) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, v := range a.Targets {
			seq.Content = append(seq.Content, scalar(v))
		}
		n.Content = append(n.Content, scalar("targets"), seq)
	}
	if len(a.AppliesTo) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, v := range a.AppliesTo {
			seq.Content = append(seq.Content, scalar(v))
		}
		n.Content = append(n.Content, scalar("applies_to"), seq)
	}
	if len(a.Inputs) > 0 {
		n.Content = append(n.Content, scalar("inputs"), inputsNode(a.Inputs))
	}
	switch c := a.Config.(type) {
	case *LaunchSessionConfig:
		if c.RepoTemplate != "" {
			add("repo_template", c.RepoTemplate)
		}
		if c.Workspace != "" {
			add("workspace", c.Workspace)
		}
		add("prompt_template", c.PromptTemplate)
		if c.Agent != "" {
			add("agent", c.Agent)
		}
		if c.PostHook != "" {
			add("post_hook", c.PostHook)
		}
		if c.PostHookTimeout != 0 {
			add("post_hook_timeout", c.PostHookTimeout.String())
		}
	case *ShellConfig:
		add("command_template", c.CommandTemplate)
		if c.Cwd != "" {
			add("cwd", c.Cwd)
		}
		if c.Timeout != 0 {
			add("timeout", c.Timeout.String())
		}
		if len(c.Env) > 0 {
			m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			keys := make([]string, 0, len(c.Env))
			for k := range c.Env {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				m.Content = append(m.Content, scalar(k), scalar(c.Env[k]))
			}
			n.Content = append(n.Content, scalar("env"), m)
		}
	case *PublishMessageConfig:
		add("message_template", c.MessageTemplate)
		add("topic", c.Topic)
	case *ClipboardConfig:
		add("text_template", c.TextTemplate)
	default:
		// A registered type (registry, actions.go) with no case here would
		// otherwise serialize with only the envelope fields above, silently
		// dropping every per-type config field on save. That is a registry/
		// writer mismatch — a programmer error, not a user data problem — so
		// this fails loudly instead of writing a truncated action.
		return nil, fmt.Errorf("actions: action %q (type %q): no actionNode writer case for config type %T; registry and writer are out of sync", a.ID, a.Type, a.Config)
	}
	return n, nil
}

func inputsNode(specs []InputSpec) *yaml.Node {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, spec := range specs {
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		add := func(k, v string) { n.Content = append(n.Content, scalar(k), scalar(v)) }
		add("name", spec.Name)
		if spec.Label != "" {
			add("label", spec.Label)
		}
		add("type", spec.Type)
		if spec.Required {
			n.Content = append(n.Content, scalar("required"), &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
		}
		if spec.Default != "" {
			add("default", spec.Default)
		}
		if spec.Placeholder != "" {
			add("placeholder", spec.Placeholder)
		}
		if len(spec.Options) > 0 {
			options := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			for _, option := range spec.Options {
				options.Content = append(options.Content, scalar(option))
			}
			n.Content = append(n.Content, scalar("options"), options)
		}
		seq.Content = append(seq.Content, n)
	}
	return seq
}

// actionFileOps isolates filesystem failures that cannot be reliably induced
// with permissions (especially under root or on platform-specific filesystems).
// It is unexported and only replaced by package tests.
type actionFileOps struct {
	mkdirAll   func(string, os.FileMode) error
	createTemp func(string, string) (*os.File, error)
	chmod      func(*os.File, os.FileMode) error
	write      func(*os.File, []byte) (int, error)
	sync       func(*os.File) error
	close      func(*os.File) error
	rename     func(string, string) error
	link       func(string, string) error
	open       func(string) (*os.File, error)
	remove     func(string) error
}

func defaultActionFileOps() actionFileOps {
	return actionFileOps{
		mkdirAll: os.MkdirAll, createTemp: os.CreateTemp,
		chmod:  func(f *os.File, mode os.FileMode) error { return f.Chmod(mode) },
		write:  func(f *os.File, data []byte) (int, error) { return f.Write(data) },
		sync:   func(f *os.File) error { return f.Sync() },
		close:  func(f *os.File) error { return f.Close() },
		rename: os.Rename, link: os.Link, open: os.Open, remove: os.Remove,
	}
}

var actionFS = defaultActionFileOps()

// installedWriteError marks an error after rename installed the target. Callers
// must reload state even though reporting the directory-sync failure.
type installedWriteError struct{ err error }

func (e *installedWriteError) Error() string { return e.err.Error() }
func (e *installedWriteError) Unwrap() error { return e.err }

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := actionFS.mkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create actions directory: %w", err)
	}
	f, err := actionFS.createTemp(dir, ".actions-*")
	if err != nil {
		return fmt.Errorf("create actions temp: %w", err)
	}
	tmp := f.Name()
	defer func() { _ = actionFS.remove(tmp) }()
	if err = actionFS.chmod(f, 0o600); err == nil {
		_, err = actionFS.write(f, data)
	}
	if err == nil {
		err = actionFS.sync(f)
	}
	if closeErr := actionFS.close(f); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write actions temp: %w", err)
	}
	if err := actionFS.rename(tmp, path); err != nil {
		return fmt.Errorf("install actions: %w", err)
	}
	d, err := actionFS.open(dir)
	if err == nil {
		err = actionFS.sync(d)
		closeErr := actionFS.close(d)
		if err == nil {
			err = closeErr
		}
	}
	if err != nil {
		return &installedWriteError{err: fmt.Errorf("sync actions directory: %w", err)}
	}
	return nil
}
