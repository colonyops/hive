package actions

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/cmd/desktop/internal/app/configmigrate"
	"github.com/colonyops/hive/cmd/desktop/internal/app/slug"
)

// Catalog is one parsed actions.yml: the actions the app runs on the user's
// behalf, and the launchers that open a terminal for them. They share a file
// but nothing else — see Launcher.
type Catalog struct {
	Actions   []Action
	Launchers []Launcher
}

// actionsFile is the top-level on-disk shape of an actions.yml document.
type actionsFile struct {
	Version   int        `yaml:"version"`
	Actions   []Action   `yaml:"actions,omitempty"`
	Launchers []Launcher `yaml:"launchers,omitempty"`
}

// LoadCatalog parses and validates path (typically desktop.ActionsPath()).
// A missing file is not an error — it reports an empty catalog, so a desktop
// install with no hand-authored actions.yml works out of the box. An
// empty-but-present file is treated the same way. Any other read error, or a
// schema/validation failure, returns a non-nil error and a zero Catalog.
func LoadCatalog(path string) (Catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Catalog{}, nil
		}
		return Catalog{}, fmt.Errorf("read actions %q: %w", path, err)
	}
	data, _, err := configmigrate.ActionsSet.Apply(raw)
	if err != nil {
		return Catalog{}, err
	}
	if data == nil {
		return Catalog{}, nil
	}
	return parseCatalog(data)
}

// parseCatalog strictly decodes the actions document, checks version ==
// configmigrate.ActionsSet.Current, and validates both lists. It is also
// called on the store's own CRUD writes (store.go) to validate current-schema
// output, so it must never route through configmigrate.ActionsSet.Apply —
// only the read path (LoadCatalog) migrates.
func parseCatalog(data []byte) (Catalog, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Catalog{}, nil
	}

	var file actionsFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil && !errors.Is(err, io.EOF) {
		return Catalog{}, fmt.Errorf("actions: %w", err)
	}

	if file.Version != configmigrate.ActionsSet.Current {
		return Catalog{}, fmt.Errorf("actions: version must be %d, got %d", configmigrate.ActionsSet.Current, file.Version)
	}

	if err := validateActions(file.Actions); err != nil {
		return Catalog{}, err
	}
	if err := validateLaunchers(file.Launchers); err != nil {
		return Catalog{}, err
	}
	return Catalog{Actions: file.Actions, Launchers: file.Launchers}, nil
}

// validateLaunchers checks each launcher and that no id repeats. Launcher ids
// live in their own namespace: nothing resolves one against the action
// catalog, so a launcher and an action may share an id without ambiguity.
func validateLaunchers(launchers []Launcher) error {
	ids := make(map[string]bool, len(launchers))
	for _, l := range launchers {
		if err := l.Validate(); err != nil {
			return err
		}
		if ids[l.ID] {
			return fmt.Errorf("launcher %q: duplicate launcher id", l.ID)
		}
		ids[l.ID] = true
	}
	return nil
}

// validateActions checks every action's envelope fields (id/label
// required, id a valid slug, no duplicate ids) and per-type config.
func validateActions(actionList []Action) error {
	ids := make(map[string]bool, len(actionList))
	for _, a := range actionList {
		if a.ID == "" {
			return fmt.Errorf("action: id is required")
		}
		if !slug.Valid(a.ID) {
			return fmt.Errorf("action %q: id is not a valid slug (lowercase letters, digits, hyphens, starting with a letter or digit, max %d chars)", a.ID, slug.MaxLen)
		}
		if ids[a.ID] {
			return fmt.Errorf("action %q: duplicate action id", a.ID)
		}
		ids[a.ID] = true

		if a.Label == "" {
			return fmt.Errorf("action %q: label is required", a.ID)
		}
		if slices.Contains(a.AppliesTo, "") {
			return fmt.Errorf("action %q: applies_to entries must not be empty", a.ID)
		}
		if err := validateInputs(a.Inputs); err != nil {
			return fmt.Errorf("action %q: %w", a.ID, err)
		}
		if a.Config == nil {
			return fmt.Errorf("action %q: no config decoded", a.ID)
		}
		if err := validateTargets(a); err != nil {
			return fmt.Errorf("action %q: %w", a.ID, err)
		}
		if err := a.Config.Validate(); err != nil {
			return fmt.Errorf("action %q (%s): %w", a.ID, a.Type, err)
		}
	}
	return nil
}

// validateTargets checks the declared surface set against the vocabulary and
// against what the action's type can actually run on, so a terminal target on
// a type that cannot serve one is refused when the catalog is authored rather
// than when its menu entry is clicked.
func validateTargets(a Action) error {
	seen := make(map[string]bool, len(a.Targets))
	for _, target := range a.Targets {
		if !targetNames[target] {
			return fmt.Errorf("unknown target %q (expected %s, %s or %s)", target, TargetItem, TargetSession, TargetWindow)
		}
		if seen[target] {
			return fmt.Errorf("duplicate target %q", target)
		}
		seen[target] = true
	}
	if a.TargetsTerminal() && !a.TerminalCapable() {
		return fmt.Errorf("type %q cannot run against a terminal session or window", a.Type)
	}
	return nil
}
