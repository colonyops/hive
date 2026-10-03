package agentws

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	configmigrate "github.com/colonyops/hive/internal/config/migrate"
)

// LoadWorkspace reads and validates one agent-workspace.yaml. Dir is set from
// path's parent directory name, so every successfully loaded Workspace knows
// which directory under the root it came from. A missing file returns an
// error wrapping os.ErrNotExist, which the store uses to distinguish "no
// manifest yet" (ignored) from a genuinely broken one (kept as an error).
func LoadWorkspace(path string) (Workspace, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Workspace{}, fmt.Errorf("read agent-workspace.yaml %s: %w", path, err)
	}
	data, _, err := configmigrate.AgentWorkspaceSet.Apply(raw)
	if err != nil {
		return Workspace{}, fmt.Errorf("agent-workspace.yaml %s: %w", path, err)
	}
	if data == nil {
		data = raw
	}
	w, err := parseWorkspace(data)
	if err != nil {
		return Workspace{}, fmt.Errorf("agent-workspace.yaml %s: %w", path, err)
	}
	return w.withDir(filepath.Base(filepath.Dir(path))), nil
}

// parseWorkspace strictly decodes an agent-workspace.yaml document already in
// hand (rather than read from disk), checks version ==
// configmigrate.AgentWorkspaceSet.Current, and validates it. It is the
// non-migrating counterpart to LoadWorkspace, for bytes that never touched a
// file — an unsaved edit checked before it is written, say.
func parseWorkspace(data []byte) (Workspace, error) {
	var w Workspace
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&w); err != nil && !errors.Is(err, io.EOF) {
		return Workspace{}, fmt.Errorf("agent-workspace.yaml: %w", err)
	}

	if w.Version != configmigrate.AgentWorkspaceSet.Current {
		return Workspace{}, fmt.Errorf("agent-workspace.yaml: version must be %d, got %d", configmigrate.AgentWorkspaceSet.Current, w.Version)
	}
	if err := w.Validate(); err != nil {
		return Workspace{}, err
	}
	return w, nil
}

// LoadLibrary reads and validates mcps.yaml at path.
func LoadLibrary(path string) (Library, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Library{}, fmt.Errorf("read mcps.yaml %s: %w", path, err)
	}
	data, _, err := configmigrate.MCPLibrarySet.Apply(raw)
	if err != nil {
		return Library{}, fmt.Errorf("mcps.yaml %s: %w", path, err)
	}
	if data == nil {
		data = raw
	}
	lib, err := parseLibrary(data)
	if err != nil {
		return Library{}, fmt.Errorf("mcps.yaml %s: %w", path, err)
	}
	return lib, nil
}

// LoadSkillLibrary reads and validates skills.yml at path.
func LoadSkillLibrary(path string) (SkillLibrary, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return SkillLibrary{}, fmt.Errorf("read skills.yml %s: %w", path, err)
	}
	data, _, err := configmigrate.SkillLibrarySet.Apply(raw)
	if err != nil {
		return SkillLibrary{}, fmt.Errorf("skills.yml %s: %w", path, err)
	}
	if data == nil {
		data = raw
	}
	lib, err := parseSkillLibrary(data)
	if err != nil {
		return SkillLibrary{}, fmt.Errorf("skills.yml %s: %w", path, err)
	}
	return lib, nil
}

// parseSkillLibrary strictly decodes a skills.yml document already in hand,
// checks version == configmigrate.SkillLibrarySet.Current, and validates it.
func parseSkillLibrary(data []byte) (SkillLibrary, error) {
	var l SkillLibrary
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&l); err != nil && !errors.Is(err, io.EOF) {
		return SkillLibrary{}, fmt.Errorf("skills.yml: %w", err)
	}

	if l.Version != configmigrate.SkillLibrarySet.Current {
		return SkillLibrary{}, fmt.Errorf("skills.yml: version must be %d, got %d", configmigrate.SkillLibrarySet.Current, l.Version)
	}
	if err := l.Validate(); err != nil {
		return SkillLibrary{}, err
	}
	return l, nil
}

// parseLibrary strictly decodes an mcps.yaml document already in hand, checks
// version == configmigrate.MCPLibrarySet.Current, and validates it.
func parseLibrary(data []byte) (Library, error) {
	var l Library
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&l); err != nil && !errors.Is(err, io.EOF) {
		return Library{}, fmt.Errorf("mcps.yaml: %w", err)
	}

	if l.Version != configmigrate.MCPLibrarySet.Current {
		return Library{}, fmt.Errorf("mcps.yaml: version must be %d, got %d", configmigrate.MCPLibrarySet.Current, l.Version)
	}
	if err := l.Validate(); err != nil {
		return Library{}, err
	}
	return l, nil
}
