// Package configmigrate upgrades an older on-disk config document to the
// version the running build expects, forward-only and one step per version.
// It operates on the file's own top-level `version:` field, not a tracking
// table, and it works on raw bytes BEFORE the strict (KnownFields) decoders in
// settings/flow/actions run, since a renamed or removed key is a hard decode
// error for them.
//
// A migration re-marshals the document from a generic map, so comments,
// formatting, and key order are NOT preserved (ADR yaml-config-migration). The comment-preserving
// normal-save writers (flow.SaveFlow, the actions store CRUD) are unaffected —
// they are never routed through here.
package configmigrate

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// Migration is one forward step. To is the version the document has AFTER
// Migrate runs (always the previous version + 1). Migrate mutates doc in place.
type Migration struct {
	To      int
	Migrate func(doc map[string]any) error
}

// Set is one file type's forward-only migration chain.
type Set struct {
	Name                string      // "settings" | "flow" | "actions" — for logs/backups
	Baseline            int         // lowest version this build can migrate
	Current             int         // the version this build reads and writes
	AllowMissingVersion bool        // when true, a missing `version:` is treated as Baseline
	Migrations          []Migration // exactly Baseline+1 .. Current, ascending, no gaps/dupes
}

// ErrVersionTooNew is returned (errors.Is-able) when a document's version
// exceeds Current: a file written by a newer build. Forward-only cannot
// downgrade it. It is a plain sentinel — configmigrate never imports package
// app and does no Kind mapping.
var ErrVersionTooNew = errors.New("config version is newer than this build supports")

// Apply reads raw, migrates it forward to Current, and returns the re-marshalled
// bytes. changed is true only when a transform ran (version < Current). When
// false, migrated is either the original bytes or nil (empty input); the caller
// decodes it but writes nothing.
//
//   - Empty input returns (nil, false, nil).
//   - Non-empty bytes that fail the lax map[string]any decode return a wrapped
//     decode error (distinct from the empty case). This is the corrupt/undecodable
//     path: migration now runs BEFORE the strict decode, so a malformed file hits
//     this lax decode first. Each loader handles it exactly as a decode failure.
//   - A version above Current returns ErrVersionTooNew and no bytes.
//   - A migration step whose Migrate closure returns an error aborts: Apply
//     returns that error and produces no bytes.
func (s Set) Apply(raw []byte) (migrated []byte, changed bool, err error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, false, nil
	}

	doc, err := decodeOneDocument(raw)
	if err != nil {
		return nil, false, fmt.Errorf("decode %s config: %w", s.Name, err)
	}

	version, err := s.version(doc)
	if err != nil {
		return nil, false, err
	}

	if version > s.Current {
		return nil, false, fmt.Errorf("%s config: version %d: %w", s.Name, version, ErrVersionTooNew)
	}
	if version == s.Current {
		return raw, false, nil
	}

	for _, step := range s.Migrations {
		if step.To <= version {
			continue
		}
		if err := step.Migrate(doc); err != nil {
			return nil, false, fmt.Errorf("migrate %s config to version %d: %w", s.Name, step.To, err)
		}
	}

	doc["version"] = s.Current

	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, false, fmt.Errorf("marshal migrated %s config: %w", s.Name, err)
	}

	return out, true, nil
}

func decodeOneDocument(raw []byte) (map[string]any, error) {
	var doc map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}
	if doc == nil {
		doc = map[string]any{}
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("multiple YAML documents are not allowed")
		}
		return nil, err
	}
	return doc, nil
}

// version reads the top-level `version:` from a lax-decoded document. A present
// value below Baseline is rejected as too old to migrate.
func (s Set) version(doc map[string]any) (int, error) {
	raw, ok := doc["version"]
	if !ok || raw == nil {
		if !s.AllowMissingVersion {
			return 0, fmt.Errorf("%s config: version field is required", s.Name)
		}
		return s.Baseline, nil
	}

	// yaml.v3 decodes a bare integer as int, falling back to int64/uint64 only
	// when the value overflows int (gopkg.in/yaml.v3 resolve.go).
	var v int
	switch n := raw.(type) {
	case int:
		v = n
	case int64:
		v = int(n)
	case uint64:
		v = int(n)
	default:
		return 0, fmt.Errorf("%s config: version field must be an integer, got %T", s.Name, raw)
	}

	if v < s.Baseline {
		return 0, fmt.Errorf("%s config: version %d is older than baseline %d", s.Name, v, s.Baseline)
	}

	return v, nil
}

// Validate asserts the chain is well-formed: Baseline <= Current, and
// Migrations in declared order are exactly Baseline+1, Baseline+2, ..., Current
// with no gaps or duplicates. A Set with Current == Baseline has zero
// Migrations and is valid. Called by a package test for every registered Set.
func (s Set) Validate() error {
	if s.Current < s.Baseline {
		return fmt.Errorf("%s: current %d is less than baseline %d", s.Name, s.Current, s.Baseline)
	}

	want := s.Baseline + 1
	for _, step := range s.Migrations {
		if step.To != want {
			return fmt.Errorf("%s: migration chain has a gap or duplicate: expected step to version %d, got %d", s.Name, want, step.To)
		}
		want++
	}

	if want-1 != s.Current {
		return fmt.Errorf("%s: migration chain reaches version %d, expected %d", s.Name, want-1, s.Current)
	}

	return nil
}
