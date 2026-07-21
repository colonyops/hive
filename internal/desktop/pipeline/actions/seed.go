package actions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const defaultActionsYAML = `version: 1
actions:
  - id: review-pr
    label: Review PR
    type: launch-session
    show_in_detail: true
    applies_to: [pr]
    repo_template: "https://github.com/{{ .Payload.repo }}.git"
    prompt_template: |
      Review pull request {{ .Payload.title }}

      {{ .Payload.url }}
  - id: start-implementation
    label: Start implementation
    type: launch-session
    show_in_detail: true
    applies_to: [issue]
    repo_template: "https://github.com/{{ .Payload.repo }}.git"
    prompt_template: |
      Work on {{ .Payload.title }}

      {{ .Payload.url }}

      {{ .Payload.body }}
`

// SeedDefaultsIfMissing installs the exact starter catalog only if path does
// not exist. It never interprets or replaces a present file, including an
// empty or invalid one. Hard-link installation is exclusive, so a concurrent
// writer that wins the race keeps its own bytes intact. The returned boolean
// reports whether this invocation installed the catalog.
func SeedDefaultsIfMissing(path string) (bool, error) {
	if _, err := os.Lstat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("stat actions seed target: %w", err)
	}
	dir := filepath.Dir(path)
	if err := actionFS.mkdirAll(dir, 0o700); err != nil {
		return false, fmt.Errorf("create actions seed directory: %w", err)
	}
	f, err := actionFS.createTemp(dir, ".actions-seed-*")
	if err != nil {
		return false, fmt.Errorf("create actions seed temp: %w", err)
	}
	tmp := f.Name()
	defer func() { _ = actionFS.remove(tmp) }()
	if err = actionFS.chmod(f, 0o600); err == nil {
		_, err = actionFS.write(f, []byte(defaultActionsYAML))
	}
	if err == nil {
		err = actionFS.sync(f)
	}
	if closeErr := actionFS.close(f); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, fmt.Errorf("write actions seed: %w", err)
	}
	if err := actionFS.link(tmp, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, fmt.Errorf("install actions seed: %w", err)
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
		return false, fmt.Errorf("sync actions seed directory: %w", err)
	}
	return true, nil
}
