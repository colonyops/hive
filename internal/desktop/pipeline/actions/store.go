package actions

import (
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
	Usage(actionID string) (ActionUsage, error)
}

// ActionStore retains its last-good snapshot if a disk reload or mutation
// candidate is invalid. All mutations re-read disk while holding this lock.
type ActionStore struct {
	path    string
	mu      sync.Mutex
	loaded  bool
	actions map[string]Action
	err     error
	usage   ActionUsageChecker
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
	out := make([]Action, 0, len(s.actions))
	for _, a := range s.actions {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *ActionStore) ViewsFor(kind string) []View {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	out := make([]View, 0, len(s.actions))
	for _, a := range s.actions {
		if a.ShowInDetail && actionAppliesTo(a, kind) {
			out = append(out, a.View())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
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

func (s *ActionStore) Get(id string) (Action, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	a, ok := s.actions[id]
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
	loaded, err := LoadActions(s.path)
	s.loaded = true
	if err != nil {
		s.err = err
		return err
	}
	s.actions = byID(loaded)
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
	for _, a := range s.actions {
		out = append(out, editableFromAction(a))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	catalog := EditableCatalog{Actions: out}
	if s.err != nil {
		catalog.Error = s.err.Error()
	}
	return catalog
}

func (s *ActionStore) GetEditable(id string) (EditableAction, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	a, ok := s.actions[id]
	return editableFromAction(a), ok
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

func (s *ActionStore) Update(id string, e EditableAction) (EditableAction, error) {
	if id != e.ID {
		return EditableAction{}, fmt.Errorf("action id is immutable")
	}
	a, err := actionFromEditable(e)
	if err != nil {
		return EditableAction{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateLocked("update", id, a)
}

func (s *ActionStore) Delete(id string) error {
	// Do not call Usage while holding s.mu: FlowStore Save/Create validate
	// action references while holding their flow lock, so doing so would invert
	// the action -> flow lock order. This preflight intentionally has the race
	// boundary documented on usageChecker.
	if checker := s.usageChecker(); checker != nil {
		usage, err := checker.Usage(id)
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
	doc, list, err := s.latestDocumentLocked()
	if err != nil {
		return err
	}
	i := findActionNode(list, id)
	if i < 0 {
		return fmt.Errorf("action %q not found", id)
	}
	list.Content = append(list.Content[:i], list.Content[i+1:]...)
	return s.writeDocumentLocked(doc)
}

func (s *ActionStore) mutateLocked(mode, id string, a Action) (EditableAction, error) {
	doc, list, err := s.latestDocumentLocked()
	if err != nil {
		return EditableAction{}, err
	}
	i := findActionNode(list, id)
	if mode == "create" {
		if i >= 0 {
			return EditableAction{}, fmt.Errorf("action %q already exists", id)
		}
		list.Content = append(list.Content, actionNode(a))
	} else {
		if i < 0 {
			return EditableAction{}, fmt.Errorf("action %q not found", id)
		}
		list.Content[i] = actionNode(a)
	}
	if err := s.writeDocumentLocked(doc); err != nil {
		return EditableAction{}, err
	}
	return editableFromAction(a), nil
}

// latestDocumentLocked rejects invalid latest disk bytes before altering disk
// or memory. Empty present files are valid and become a new v1 document.
func (s *ActionStore) latestDocumentLocked() (*yaml.Node, *yaml.Node, error) {
	data, err := os.ReadFile(s.path)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("read actions %q: %w", s.path, err)
	}
	if err == nil {
		if _, err := parseActions(data); err != nil {
			return nil, nil, fmt.Errorf("actions file changed to invalid content: %w", err)
		}
	}
	if os.IsNotExist(err) || len(strings.TrimSpace(string(data))) == 0 {
		doc := newDocument()
		return doc, doc.Content[0].Content[3], nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("parse actions document: %w", err)
	}
	root := doc.Content[0]
	var list *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "actions" {
			list = root.Content[i+1]
			break
		}
	}
	if list == nil || list.Kind != yaml.SequenceNode {
		return nil, nil, fmt.Errorf("actions: valid document missing actions sequence")
	}
	return &doc, list, nil
}

func newDocument() *yaml.Node {
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	root.Content = []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: "version"}, {Kind: yaml.ScalarNode, Tag: "!!int", Value: "1"}, {Kind: yaml.ScalarNode, Tag: "!!str", Value: "actions"}, {Kind: yaml.SequenceNode, Tag: "!!seq"}}
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
}

func findActionNode(list *yaml.Node, id string) int {
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
	if _, err := parseActions(data); err != nil {
		return fmt.Errorf("validate action change: %w", err)
	}
	if err := atomicWrite(s.path, data); err != nil {
		var installed *installedWriteError
		if errors.As(err, &installed) {
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

func actionNode(a Action) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	add := func(k, v string) { n.Content = append(n.Content, scalar(k), scalar(v)) }
	add("id", a.ID)
	add("label", a.Label)
	add("type", a.Type)
	if a.ShowInDetail {
		n.Content = append(n.Content, scalar("show_in_detail"), &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
	}
	if len(a.AppliesTo) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, v := range a.AppliesTo {
			seq.Content = append(seq.Content, scalar(v))
		}
		n.Content = append(n.Content, scalar("applies_to"), seq)
	}
	switch c := a.Config.(type) {
	case *LaunchSessionConfig:
		if c.RepoTemplate != "" {
			add("repo_template", c.RepoTemplate)
		}
		add("prompt_template", c.PromptTemplate)
		if c.Agent != "" {
			add("agent", c.Agent)
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
	}
	return n
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
