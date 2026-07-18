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
// ownership: source and profile definitions live in a user-editable YAML
// config file (dotfiles territory — see desktop.ConfigPath), app-local read
// state lives in the state dir (readstate.json). On config errors the
// last-good definitions are retained so a half-saved edit degrades instead of
// blanking the app.
type Store struct {
	configPath string
	stateDir   string

	mu           sync.Mutex
	sources      []SourceDef
	profiles     []ProfileDef
	configLoaded bool
	readState    map[string]time.Time
	stateLoaded  bool
}

func NewStore(configPath, stateDir string) *Store {
	return &Store{configPath: configPath, stateDir: stateDir}
}

// DefaultSources are the sources seeded alongside a new profile when they do
// not exist yet: authored PRs, cross-repo assignments, and the notifications
// inbox.
func DefaultSources() []SourceDef {
	return []SourceDef{
		{ID: "my-prs", Kind: "search", Query: "is:open is:pr author:@me archived:false"},
		{ID: "assigned", Kind: "search", Query: "is:open assignee:@me archived:false"},
		{ID: "inbox", Kind: "notifications"},
	}
}

// DefaultFeeds are the feeds seeded into a new profile, one per default
// source.
func DefaultFeeds() []FeedDef {
	return []FeedDef{
		{ID: "my-open-prs", Name: "My open PRs", Sources: []string{"my-prs"}},
		{ID: "notifications-inbox", Name: "Notifications inbox", Sources: []string{"inbox"}},
		{ID: "assigned-across-org", Name: "Assigned across org", Sources: []string{"assigned"}},
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

// Sources returns the source definitions from the config file, with the same
// last-good semantics as Profiles.
func (s *Store) Sources() ([]SourceDef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadConfigLocked(false); err != nil {
		return nil, err
	}
	out := make([]SourceDef, len(s.sources))
	copy(out, s.sources)
	return out, nil
}

// FeedDefFor returns one feed's definition, for edit prefill.
func (s *Store) FeedDefFor(profileID, feedID string) (FeedDef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadConfigLocked(false); err != nil {
		return FeedDef{}, err
	}
	for _, profile := range s.profiles {
		if profile.ID != profileID {
			continue
		}
		for _, feedDef := range profile.Feeds {
			if feedDef.ID == feedID {
				return feedDef, nil
			}
		}
		return FeedDef{}, fmt.Errorf("feed: unknown feed %q in profile %q", feedID, profileID)
	}
	return FeedDef{}, fmt.Errorf("feed: unknown profile %q", profileID)
}

// Reload re-reads the config file, retaining the last-good definitions when
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
// config file, preserving any hand-written comments and formatting. Default
// sources the feeds reference are appended first when absent. It refuses to
// touch a config that currently fails to parse: appending to a broken
// document could compound the damage.
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

	updated, err := s.readConfigBytesLocked()
	if err != nil {
		return ProfileDef{}, err
	}
	// Seed missing default sources by ID. An existing source with a default
	// ID is left as the user defined it; the seeded feed then reads from it.
	existing := make(map[string]bool, len(s.sources))
	for _, src := range s.sources {
		existing[src.ID] = true
	}
	for _, src := range DefaultSources() {
		if existing[src.ID] {
			continue
		}
		if updated, err = appendSourceToConfig(updated, src); err != nil {
			return ProfileDef{}, fmt.Errorf("feed: %w", err)
		}
	}
	if updated, err = appendProfileToConfig(updated, def); err != nil {
		return ProfileDef{}, fmt.Errorf("feed: %w", err)
	}
	if err := s.commitConfigLocked(updated); err != nil {
		return ProfileDef{}, err
	}
	return def, nil
}

// CreateSource appends a source to the config file. The caller-supplied ID is
// slugified and uniquified (-2 suffix) against existing source IDs.
func (s *Store) CreateSource(def SourceDef) (SourceDef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadConfigLocked(true); err != nil {
		return SourceDef{}, fmt.Errorf("feed: fix %s before creating sources: %w", filepath.Base(s.configPath), err)
	}

	slug := slugify(def.ID)
	if slug == "" {
		return SourceDef{}, fmt.Errorf("feed: source id is empty")
	}
	taken := make(map[string]bool, len(s.sources))
	for _, src := range s.sources {
		taken[src.ID] = true
	}
	def.ID = uniqueID(slug, taken)
	if err := validateSource(def); err != nil {
		return SourceDef{}, fmt.Errorf("feed: %w", err)
	}

	data, err := s.readConfigBytesLocked()
	if err != nil {
		return SourceDef{}, err
	}
	updated, err := appendSourceToConfig(data, def)
	if err != nil {
		return SourceDef{}, fmt.Errorf("feed: %w", err)
	}
	if err := s.commitConfigLocked(updated); err != nil {
		return SourceDef{}, err
	}
	return def, nil
}

// CreateFeed appends a feed to the profile. The feed ID is derived from the
// name, uniquified (-2 suffix) against the profile's existing feed IDs.
func (s *Store) CreateFeed(profileID string, def FeedDef) (FeedDef, error) {
	def.Name = strings.TrimSpace(def.Name)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadConfigLocked(true); err != nil {
		return FeedDef{}, fmt.Errorf("feed: fix %s before creating feeds: %w", filepath.Base(s.configPath), err)
	}

	profile, ok := s.profileLocked(profileID)
	if !ok {
		return FeedDef{}, fmt.Errorf("feed: unknown profile %q", profileID)
	}
	slug := slugify(def.Name)
	if slug == "" {
		slug = "feed"
	}
	taken := make(map[string]bool, len(profile.Feeds))
	for _, feedDef := range profile.Feeds {
		taken[feedDef.ID] = true
	}
	def.ID = uniqueID(slug, taken)

	data, err := s.readConfigBytesLocked()
	if err != nil {
		return FeedDef{}, err
	}
	updated, err := appendFeedToConfig(data, profileID, def)
	if err != nil {
		return FeedDef{}, fmt.Errorf("feed: %w", err)
	}
	if err := s.commitConfigLocked(updated); err != nil {
		return FeedDef{}, err
	}
	return def, nil
}

// UpdateFeed replaces the feed's definition in place; the feed keeps its ID.
func (s *Store) UpdateFeed(profileID, feedID string, def FeedDef) error {
	def.ID = feedID
	def.Name = strings.TrimSpace(def.Name)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadConfigLocked(true); err != nil {
		return fmt.Errorf("feed: fix %s before editing feeds: %w", filepath.Base(s.configPath), err)
	}

	data, err := s.readConfigBytesLocked()
	if err != nil {
		return err
	}
	updated, err := updateFeedInConfig(data, profileID, feedID, def)
	if err != nil {
		return fmt.Errorf("feed: %w", err)
	}
	return s.commitConfigLocked(updated)
}

// DeleteFeed removes the feed from the profile; the profile and its other
// feeds are untouched.
func (s *Store) DeleteFeed(profileID, feedID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadConfigLocked(true); err != nil {
		return fmt.Errorf("feed: fix %s before deleting feeds: %w", filepath.Base(s.configPath), err)
	}

	data, err := s.readConfigBytesLocked()
	if err != nil {
		return err
	}
	updated, err := deleteFeedFromConfig(data, profileID, feedID)
	if err != nil {
		return fmt.Errorf("feed: %w", err)
	}
	return s.commitConfigLocked(updated)
}

// DeleteProfile removes the profile and its feeds from the config. Sources
// are left untouched: they are shared, decoupled definitions other profiles
// may still reference. Deleting the last profile is allowed — it leaves an
// empty profiles list, the same state a fresh install starts from.
func (s *Store) DeleteProfile(profileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadConfigLocked(true); err != nil {
		return fmt.Errorf("feed: fix %s before deleting profiles: %w", filepath.Base(s.configPath), err)
	}

	data, err := s.readConfigBytesLocked()
	if err != nil {
		return err
	}
	updated, err := deleteProfileFromConfig(data, profileID)
	if err != nil {
		return fmt.Errorf("feed: %w", err)
	}
	return s.commitConfigLocked(updated)
}

// readConfigBytesLocked reads the raw config file; a missing file reads as
// empty, matching the append helpers' empty-document path.
func (s *Store) readConfigBytesLocked() ([]byte, error) {
	data, err := os.ReadFile(s.configPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("feed: read config: %w", err)
	}
	return data, nil
}

// commitConfigLocked validates the updated document, writes it atomically,
// and adopts the parsed result as the in-memory state. A config that fails
// validation is never written.
func (s *Store) commitConfigLocked(updated []byte) error {
	file, err := parseConfig(updated)
	if err != nil {
		return fmt.Errorf("feed: refusing to write invalid config: %w", err)
	}
	if err := writeFileAtomic(s.configPath, updated); err != nil {
		return err
	}
	s.sources = file.Sources
	s.profiles = file.Profiles
	s.configLoaded = true
	return nil
}

func (s *Store) profileLocked(profileID string) (ProfileDef, bool) {
	for _, profile := range s.profiles {
		if profile.ID == profileID {
			return profile, true
		}
	}
	return ProfileDef{}, false
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
	return uniqueID(slug, taken)
}

// uniqueID suffixes slug with -2, -3, … until it is not taken.
func uniqueID(slug string, taken map[string]bool) string {
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
// the last-good definitions in place — the returned error is the caller's to
// surface (the watcher pushes it to the frontend).
func (s *Store) loadConfigLocked(force bool) error {
	if s.configLoaded && !force {
		return nil
	}
	data, err := os.ReadFile(s.configPath)
	if os.IsNotExist(err) {
		data, err = nil, nil
	}
	var file configFile
	if err == nil {
		file, err = parseConfig(data)
	}
	if err != nil {
		return fmt.Errorf("feed: %s: %w", filepath.Base(s.configPath), err)
	}
	s.sources = file.Sources
	s.profiles = file.Profiles
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
