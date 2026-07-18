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

	"github.com/google/uuid"
)

// FeedDef is a persisted feed definition inside a profile. The vocabulary
// (kind + query) deliberately mirrors the saved-view schema proposed for TUI
// sources (issue #370) so the two can converge on one definition later.
type FeedDef struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"` // "search" | "notifications"
	Query string `json:"query,omitempty"`
}

// ProfileDef is a persisted profile ("workspace") definition.
type ProfileDef struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Feeds []FeedDef `json:"feeds"`
}

// Store persists desktop feed state under one directory: profile
// definitions (profiles.json) and app-local read state (readstate.json).
type Store struct {
	dir string

	mu        sync.Mutex
	profiles  []ProfileDef
	readState map[string]time.Time
	loaded    bool
}

func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// DefaultFeeds are the feeds seeded into a new profile, matching the mock
// roadmap: authored PRs, the notifications inbox, and cross-repo assignments.
func DefaultFeeds() []FeedDef {
	return []FeedDef{
		{ID: "my-open-prs", Name: "My open PRs", Kind: "search", Query: "is:open is:pr author:@me archived:false"},
		{ID: "notifications-inbox", Name: "Notifications inbox", Kind: "notifications"},
		{ID: "assigned-across-org", Name: "Assigned across org", Kind: "search", Query: "is:open assignee:@me archived:false"},
	}
}

// Profiles returns the persisted profile definitions.
func (s *Store) Profiles() ([]ProfileDef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return nil, err
	}
	out := make([]ProfileDef, len(s.profiles))
	copy(out, s.profiles)
	return out, nil
}

// CreateProfile persists a new profile with the default feeds.
func (s *Store) CreateProfile(name string) (ProfileDef, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ProfileDef{}, fmt.Errorf("feed: profile name is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return ProfileDef{}, err
	}

	def := ProfileDef{
		ID:    uuid.NewString(),
		Name:  name,
		Feeds: DefaultFeeds(),
	}
	s.profiles = append(s.profiles, def)
	if err := s.saveProfiles(); err != nil {
		return ProfileDef{}, err
	}
	return def, nil
}

// ReadAt returns when the item was last marked read.
func (s *Store) ReadAt(itemID string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
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
	if err := s.load(); err != nil {
		return err
	}
	if s.readState == nil {
		s.readState = make(map[string]time.Time)
	}
	s.readState[itemID] = updatedAt
	return s.saveReadState()
}

func (s *Store) profilesPath() string  { return filepath.Join(s.dir, "profiles.json") }
func (s *Store) readStatePath() string { return filepath.Join(s.dir, "readstate.json") }

func (s *Store) load() error {
	if s.loaded {
		return nil
	}
	if err := readJSONFile(s.profilesPath(), &s.profiles); err != nil {
		return err
	}
	if err := readJSONFile(s.readStatePath(), &s.readState); err != nil {
		return err
	}
	s.loaded = true
	return nil
}

func (s *Store) saveProfiles() error {
	return writeJSONFile(s.profilesPath(), s.profiles)
}

func (s *Store) saveReadState() error {
	return writeJSONFile(s.readStatePath(), s.readState)
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

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("feed: create state dir: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("feed: encode %s: %w", filepath.Base(path), err)
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
