package feed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Store persists desktop feed data across two locations with different
// ownership: profile definitions live in a user-editable YAML config file
// (dotfiles territory — see desktop.ConfigPath), app-local read state lives
// in the state dir (readstate.json). On config errors the last-good
// profiles are retained so a half-saved edit degrades instead of blanking
// the app.
type Store struct {
	configPath string
	stateDir   string

	mu           sync.Mutex
	profiles     []ProfileDef
	configLoaded bool
	readState    map[string]time.Time
	stateLoaded  bool
}

func NewStore(configPath, stateDir string) *Store {
	return &Store{configPath: configPath, stateDir: stateDir}
}

// DefaultFeeds are the feeds seeded into a new profile: authored PRs, the
// notifications inbox, and cross-repo assignments.
func DefaultFeeds() []FeedDef {
	return []FeedDef{
		{ID: "my-open-prs", Name: "My open PRs", Kind: "search", Query: "is:open is:pr author:@me archived:false"},
		{ID: "notifications-inbox", Name: "Notifications inbox", Kind: "notifications"},
		{ID: "assigned-across-org", Name: "Assigned across org", Kind: "search", Query: "is:open assignee:@me archived:false"},
	}
}

// Profiles returns the profile definitions from the config file. When the
// config is broken and no prior good load exists, the parse error is
// returned so the failure is visible instead of looking like a fresh
// install (which would route the user into onboarding).
func (s *Store) Profiles() ([]ProfileDef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadConfigLocked(false); err != nil {
		return nil, err
	}
	out := make([]ProfileDef, len(s.profiles))
	copy(out, s.profiles)
	return out, nil
}

// Reload re-reads the config file, retaining the last-good profiles when
// the new content fails to parse or validate.
func (s *Store) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadConfigLocked(true)
}

// ConfigInfo reports the config file's location and current content for the
// frontend, parsing the on-disk bytes fresh so the verdict reflects what
// the user's editor just saved, not a cached state.
func (s *Store) ConfigInfo() ConfigInfo {
	info := ConfigInfo{Path: s.configPath, Valid: true}
	data, err := os.ReadFile(s.configPath)
	switch {
	case os.IsNotExist(err):
		info.YAML = ExampleConfig()
	case err != nil:
		info.Valid = false
		info.Error = err.Error()
	default:
		info.Exists = true
		info.YAML = string(data)
		if _, parseErr := parseConfig(data); parseErr != nil {
			info.Valid = false
			info.Error = parseErr.Error()
		}
	}
	return info
}

// CreateProfile appends a profile seeded with the default feeds to the
// config file, preserving any hand-written comments and formatting. It
// refuses to touch a config that currently fails to parse: appending to a
// broken document could compound the damage.
func (s *Store) CreateProfile(name string) (ProfileDef, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ProfileDef{}, fmt.Errorf("feed: profile name is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadConfigLocked(true); err != nil {
		return ProfileDef{}, fmt.Errorf("feed: fix %s before creating profiles: %w", filepath.Base(s.configPath), err)
	}

	def := ProfileDef{
		ID:    s.uniqueProfileID(slugify(name)),
		Name:  name,
		Feeds: DefaultFeeds(),
	}

	data, err := os.ReadFile(s.configPath)
	if err != nil && !os.IsNotExist(err) {
		return ProfileDef{}, fmt.Errorf("feed: read config: %w", err)
	}
	updated, err := appendProfileToConfig(data, def)
	if err != nil {
		return ProfileDef{}, fmt.Errorf("feed: %w", err)
	}
	if err := writeFileAtomic(s.configPath, updated); err != nil {
		return ProfileDef{}, err
	}
	s.profiles = append(s.profiles, def)
	return def, nil
}

// uniqueProfileID suffixes the slug until it is unused, so two profiles
// named "Work" do not collide.
func (s *Store) uniqueProfileID(slug string) string {
	if slug == "" {
		slug = "profile"
	}
	taken := make(map[string]bool, len(s.profiles))
	for _, def := range s.profiles {
		taken[def.ID] = true
	}
	id := slug
	for n := 2; taken[id]; n++ {
		id = fmt.Sprintf("%s-%d", slug, n)
	}
	return id
}

// ReadAt returns when the item was last marked read.
func (s *Store) ReadAt(itemID string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadStateLocked(); err != nil {
		return time.Time{}, false
	}
	at, ok := s.readState[itemID]
	return at, ok
}

// MarkRead records the item as read as of updatedAt: the item shows unread
// again if it is updated later.
func (s *Store) MarkRead(itemID string, updatedAt time.Time) error {
	if itemID == "" {
		return fmt.Errorf("feed: item ID is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadStateLocked(); err != nil {
		return err
	}
	if s.readState == nil {
		s.readState = make(map[string]time.Time)
	}
	s.readState[itemID] = updatedAt
	return s.saveReadState()
}

func (s *Store) readStatePath() string { return filepath.Join(s.stateDir, "readstate.json") }

// loadConfigLocked reads and parses the config file. Once a load has
// succeeded, non-forced calls are no-ops and a failed forced reload leaves
// the last-good profiles in place — the returned error is the caller's to
// surface (the watcher pushes it to the frontend).
func (s *Store) loadConfigLocked(force bool) error {
	if s.configLoaded && !force {
		return nil
	}
	data, err := os.ReadFile(s.configPath)
	if os.IsNotExist(err) {
		data, err = nil, nil
	}
	var defs []ProfileDef
	if err == nil {
		defs, err = parseConfig(data)
	}
	if err != nil {
		return fmt.Errorf("feed: %s: %w", filepath.Base(s.configPath), err)
	}
	s.profiles = defs
	s.configLoaded = true
	return nil
}

func (s *Store) loadStateLocked() error {
	if s.stateLoaded {
		return nil
	}
	if err := readJSONFile(s.readStatePath(), &s.readState); err != nil {
		return err
	}
	s.stateLoaded = true
	return nil
}

func (s *Store) saveReadState() error {
	data, err := json.MarshalIndent(s.readState, "", "  ")
	if err != nil {
		return fmt.Errorf("feed: encode read state: %w", err)
	}
	return writeFileAtomic(s.readStatePath(), data)
}

func readJSONFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("feed: read %s: %w", filepath.Base(path), err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("feed: parse %s: %w", filepath.Base(path), err)
	}
	return nil
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("feed: create %s dir: %w", filepath.Base(path), err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("feed: write %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("feed: replace %s: %w", filepath.Base(path), err)
	}
	return nil
}

// ProfileLetter is the rail avatar letter for a profile name.
func ProfileLetter(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "?"
}
