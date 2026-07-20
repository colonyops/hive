package actions

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// actionsFile is the top-level on-disk shape of an actions.yml document.
type actionsFile struct {
	Version int      `yaml:"version"`
	Actions []Action `yaml:"actions,omitempty"`
}

// LoadActions parses and validates path (typically desktop.ActionsPath()).
// A missing file is not an error — it reports an empty action set, so a
// desktop install with no hand-authored actions.yml works out of the box.
// An empty-but-present file is treated the same way. Any other read error,
// or a schema/validation failure, returns a non-nil error and a nil slice.
func LoadActions(path string) ([]Action, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read actions %q: %w", path, err)
	}
	return parseActions(data)
}

// parseActions strictly decodes the actions document, checks version == 1,
// and runs validateActions.
func parseActions(data []byte) ([]Action, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}

	var file actionsFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("actions: %w", err)
	}

	if file.Version != 1 {
		return nil, fmt.Errorf("actions: version must be 1, got %d", file.Version)
	}

	if err := validateActions(file.Actions); err != nil {
		return nil, err
	}
	return file.Actions, nil
}

// validateActions checks every action's envelope fields (id/label
// required, id a valid slug, no duplicate ids) and per-type config.
func validateActions(actionList []Action) error {
	ids := make(map[string]bool, len(actionList))
	for _, a := range actionList {
		if a.ID == "" {
			return fmt.Errorf("action: id is required")
		}
		if !validSlug(a.ID) {
			return fmt.Errorf("action %q: id is not a valid slug (lowercase letters, digits, hyphens, starting with a letter or digit, max %d chars)", a.ID, maxSlugLen)
		}
		if ids[a.ID] {
			return fmt.Errorf("action %q: duplicate action id", a.ID)
		}
		ids[a.ID] = true

		if a.Label == "" {
			return fmt.Errorf("action %q: label is required", a.ID)
		}
		for _, kind := range a.AppliesTo {
			if kind == "" {
				return fmt.Errorf("action %q: applies_to entries must not be empty", a.ID)
			}
		}
		if a.Config == nil {
			return fmt.Errorf("action %q: no config decoded", a.ID)
		}
		if err := a.Config.Validate(); err != nil {
			return fmt.Errorf("action %q (%s): %w", a.ID, a.Type, err)
		}
	}
	return nil
}
