package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"

	"github.com/zalando/go-keyring"
)

const (
	// EnvKeyringService gives development launches an isolated OS keychain namespace.
	EnvKeyringService = "HIVE_DESKTOP_DEVELOPMENT_KEYRING_SERVICE"
	keyringService    = "sh.hive.desktop"
)

// KeychainStore keeps values in the OS keychain and refs in a JSON index
// beside the app's other state.
//
// The index exists because keychains do not enumerate: zalando/go-keyring
// has no List, and neither does the API underneath it. Only refs go in the
// file — never values — so a leaked or backed-up index discloses which
// accounts are configured and nothing more.
type KeychainStore struct {
	mu        sync.Mutex
	indexPath string
	service   string
}

// NewKeychainStore builds a store whose ref index lives at indexPath. The
// path is a parameter rather than derived from internal/app/settings so this
// package stays a leaf and a test can point it at a temp dir.
func NewKeychainStore(indexPath string) *KeychainStore {
	return NewKeychainStoreWithService(indexPath, keyringService)
}

// NewKeychainStoreWithService builds a store in a specific OS keychain namespace.
func NewKeychainStoreWithService(indexPath, service string) *KeychainStore {
	if service == "" {
		service = keyringService
	}
	return &KeychainStore{indexPath: indexPath, service: service}
}

// Get reads one credential. A ref listed in the index whose keychain entry is
// gone — revoked in Keychain Access, restored from a backup without it — is
// pruned and reported as absent: the index is a cache and the keychain is the
// truth, and the alternative is a "Connected" badge over nothing.
func (s *KeychainStore) Get(ref Ref) (string, error) {
	if err := ref.Validate(); err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	value, err := keyring.Get(s.service, ref.String())
	if errors.Is(err, keyring.ErrNotFound) {
		// Best-effort: failing to prune must not turn a missing credential
		// into an error the caller has to distinguish from ErrNotFound.
		_ = s.removeFromIndex(ref)
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("credentials: read %s from keychain: %w", ref, err)
	}
	return value, nil
}

// Set stores a credential and records its ref. The keychain is written first:
// an index entry with no keychain value is the divergence Get already
// self-heals, whereas a keychain value with no index entry is invisible to
// List and leaks silently.
func (s *KeychainStore) Set(ref Ref, value string) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if value == "" {
		return fmt.Errorf("credentials: refusing to store an empty value for %s", ref)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := keyring.Set(s.service, ref.String(), value); err != nil {
		return fmt.Errorf("credentials: write %s to keychain: %w", ref, err)
	}
	return s.addToIndex(ref)
}

// Delete removes a credential and its ref. A ref absent from either side is
// not an error: deleting what is already gone is the caller's intent either
// way.
func (s *KeychainStore) Delete(ref Ref) error {
	if err := ref.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	err := keyring.Delete(s.service, ref.String())
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("credentials: delete %s from keychain: %w", ref, err)
	}
	return s.removeFromIndex(ref)
}

// List returns every recorded ref, sorted.
//
// It reads the index alone and does not verify each ref against the keychain:
// on macOS every keychain read can prompt, and a status screen that
// enumerates connectors must not turn into one prompt per provider. Callers
// that need certainty about a single credential use Get, which does check.
func (s *KeychainStore) List() ([]Ref, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readIndex()
}

// ── The ref index ────────────────────────────────────────────────────────────

// index is the on-disk shape. A wrapper object rather than a bare array so a
// later field (a schema version, a per-ref label) does not have to change the
// file's top-level type.
type index struct {
	Refs []string `json:"refs"`
}

// readIndex loads the ref index. A missing file is an empty index, not an
// error: that is the state on first run.
//
// A malformed entry is skipped rather than failing the read. The index is a
// cache — refusing to list any credential because one line is corrupt would
// lock a user out of the credentials that are fine.
func (s *KeychainStore) readIndex() ([]Ref, error) {
	data, err := os.ReadFile(s.indexPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("credentials: read ref index: %w", err)
	}

	var idx index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("credentials: parse ref index %s: %w", s.indexPath, err)
	}

	refs := make([]Ref, 0, len(idx.Refs))
	for _, raw := range idx.Refs {
		ref, err := ParseRef(raw)
		if err != nil {
			continue
		}
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].String() < refs[j].String() })
	return refs, nil
}

// writeIndex replaces the index atomically. The temp file is created in the
// destination directory so the rename stays within one filesystem, and the
// mode is 0600 because the set of configured accounts is nobody else's
// business.
func (s *KeychainStore) writeIndex(refs []Ref) error {
	dir := filepath.Dir(s.indexPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("credentials: create state dir: %w", err)
	}

	sort.Slice(refs, func(i, j int) bool { return refs[i].String() < refs[j].String() })
	raw := make([]string, 0, len(refs))
	for _, ref := range refs {
		raw = append(raw, ref.String())
	}
	data, err := json.MarshalIndent(index{Refs: raw}, "", "  ")
	if err != nil {
		return fmt.Errorf("credentials: encode ref index: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".credentials-*.json")
	if err != nil {
		return fmt.Errorf("credentials: create temp ref index: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once the rename succeeds

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("credentials: chmod temp ref index: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("credentials: write temp ref index: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("credentials: close temp ref index: %w", err)
	}
	if err := os.Rename(tmpName, s.indexPath); err != nil {
		return fmt.Errorf("credentials: replace ref index: %w", err)
	}
	return nil
}

// addToIndex records a ref, leaving the file untouched when it is already
// present so a re-Set of an existing credential does no disk write.
func (s *KeychainStore) addToIndex(ref Ref) error {
	refs, err := s.readIndex()
	if err != nil {
		return err
	}
	if slices.Contains(refs, ref) {
		return nil
	}
	return s.writeIndex(append(refs, ref))
}

// removeFromIndex drops a ref, leaving the file untouched when it is not
// there.
func (s *KeychainStore) removeFromIndex(ref Ref) error {
	refs, err := s.readIndex()
	if err != nil {
		return err
	}
	kept := make([]Ref, 0, len(refs))
	for _, existing := range refs {
		if existing != ref {
			kept = append(kept, existing)
		}
	}
	if len(kept) == len(refs) {
		return nil
	}
	return s.writeIndex(kept)
}
