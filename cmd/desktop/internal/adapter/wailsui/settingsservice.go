package wailsui

import (
	"context"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/fonts"
)

// SettingsService exposes user-tunable settings to the frontend. The JSON
// shapes below are the wire contract; the core deals in Durations and plain
// maps.
type SettingsService struct {
	settings *app.SettingsService
	fonts    *fonts.Lister
}

func NewSettingsService(s *app.SettingsService) *SettingsService {
	return &SettingsService{settings: s, fonts: fonts.NewLister()}
}

// GithubSettings is the GitHub integration's editable configuration. Seconds
// rather than a duration string: this is the wire shape the frontend edits.
type GithubSettings struct {
	PollIntervalSeconds    int `json:"pollIntervalSeconds"`
	MinPollIntervalSeconds int `json:"minPollIntervalSeconds"`
}

// NotificationSettings is the desktop notification configuration. Delivery is
// carried as a resolved string from the closed set auto/system/app.
type NotificationSettings struct {
	NotificationsEnabled bool `json:"notificationsEnabled"`
	// Delivery is where an eligible notification is surfaced: "auto" (an OS
	// banner only while Hive is unfocused), "system" (always an OS banner), or
	// "app" (always in-app).
	Delivery          string `json:"delivery"`
	NotificationSound bool   `json:"notificationSound"`
}

// AppearanceSettings is the frontend's presentation configuration. String
// values are carried verbatim: the frontend owns each valid set and heals
// unknown values, so an empty field means "nothing persisted yet" rather than
// an error.
type AppearanceSettings struct {
	Theme string `json:"theme"`
	// FontFamily is the family the app's chrome draws with and MonoFontFamily
	// the one its monospace text draws with. Empty is the bundled face; a
	// generic keyword (system-ui, ui-monospace) is the platform stack.
	FontFamily         string `json:"fontFamily"`
	MonoFontFamily     string `json:"monoFontFamily"`
	TerminalFontSizePx int    `json:"terminalFontSizePx"`
	// TerminalFontFamily is an installed monospace family for the terminal
	// alone; empty is the bundled face.
	TerminalFontFamily string `json:"terminalFontFamily"`
	// The CSS weights normal and bold cells draw at. Zero means nothing
	// persisted — the frontend owns the defaults and heals anything else.
	TerminalFontWeight     int `json:"terminalFontWeight"`
	TerminalFontWeightBold int `json:"terminalFontWeightBold"`
	// TerminalLineHeight multiplies the cell height; TerminalLetterSpacing
	// widens the cell by whole device pixels. Zero means nothing persisted.
	TerminalLineHeight    float64 `json:"terminalLineHeight"`
	TerminalLetterSpacing int     `json:"terminalLetterSpacing"`
	// TerminalShowWindows lists every active session's tmux windows in the
	// terminal sidebar, not just the attached session's. Ships on.
	TerminalShowWindows bool `json:"terminalShowWindows"`
	// TerminalShowStatusBar gives the attached session a status bar carrying
	// its checkout's git and pull-request state. Ships off.
	TerminalShowStatusBar bool `json:"terminalShowStatusBar"`
	// TerminalPoolSize is how many sessions the terminal view keeps attached
	// for instant switching (ADR terminal-attach-pool). Carried verbatim; the frontend heals
	// anything outside 1-6 to the default, 3.
	TerminalPoolSize int `json:"terminalPoolSize"`
	// Canvas typography is stored as preset names. The frontend owns the maps
	// from those names to CSS values and heals unknown values.
	CanvasFontSize    string `json:"canvasFontSize"`
	CanvasLineSpacing string `json:"canvasLineSpacing"`
	CanvasPageWidth   string `json:"canvasPageWidth"`
}

// KeybindingSettings carries keyboard shortcut overrides keyed by command id.
// Like AppearanceSettings the values are opaque to Go: the frontend owns the
// command vocabulary and the combo grammar.
//
// An id absent from Overrides keeps its catalog default; an id mapped to an
// empty list is explicitly unbound.
type KeybindingSettings struct {
	Overrides map[string][]string `json:"overrides"`
}

func (s *SettingsService) KeybindingSettings(ctx context.Context) (KeybindingSettings, error) {
	overrides, err := s.settings.Keybindings(ctx)
	if err != nil {
		return KeybindingSettings{}, err
	}
	return KeybindingSettings{Overrides: overrides}, nil
}

func (s *SettingsService) SetKeybindingSettings(ctx context.Context, in KeybindingSettings) error {
	return s.settings.SetKeybindings(ctx, in.Overrides)
}

func (s *SettingsService) AppearanceSettings(ctx context.Context) (AppearanceSettings, error) {
	current, err := s.settings.Appearance(ctx)
	if err != nil {
		return AppearanceSettings{}, err
	}
	return AppearanceSettings{
		Theme:                  current.Theme,
		FontFamily:             current.FontFamily,
		MonoFontFamily:         current.MonoFontFamily,
		TerminalFontSizePx:     current.TerminalFontSizePx,
		TerminalFontFamily:     current.TerminalFontFamily,
		TerminalFontWeight:     current.TerminalFontWeight,
		TerminalFontWeightBold: current.TerminalFontWeightBold,
		TerminalLineHeight:     current.TerminalLineHeight,
		TerminalLetterSpacing:  current.TerminalLetterSpacing,
		TerminalShowWindows:    current.TerminalShowWindows,
		TerminalShowStatusBar:  current.TerminalShowStatusBar,
		TerminalPoolSize:       current.TerminalPoolSize,
		CanvasFontSize:         current.CanvasFontSize,
		CanvasLineSpacing:      current.CanvasLineSpacing,
		CanvasPageWidth:        current.CanvasPageWidth,
	}, nil
}

// InstalledFonts is what the font pickers offer: every installed family, and
// the fixed-pitch subset the ones that draw a grid are limited to.
type InstalledFonts struct {
	All       []string `json:"all"`
	Monospace []string `json:"monospace"`
}

// Fonts lists the families installed on this machine, for the app and terminal
// font pickers. The webview cannot enumerate them itself — queryLocalFonts is
// Chromium-only and macOS runs on WKWebView.
//
// The scan is cached for the process, so a font installed while the app runs
// appears on the next launch.
func (s *SettingsService) Fonts(context.Context) (InstalledFonts, error) {
	families := s.fonts.List()
	return InstalledFonts{All: families.All, Monospace: families.Monospace}, nil
}

// The appearance setters are per-field so the theme picker and the font pickers
// cannot clobber each other's persisted value.
func (s *SettingsService) SetTheme(ctx context.Context, theme string) error {
	return s.settings.SetTheme(ctx, theme)
}

func (s *SettingsService) SetFontFamily(ctx context.Context, family string) error {
	return s.settings.SetFontFamily(ctx, family)
}

func (s *SettingsService) SetMonoFontFamily(ctx context.Context, family string) error {
	return s.settings.SetMonoFontFamily(ctx, family)
}

func (s *SettingsService) SetTerminalFontSize(ctx context.Context, px int) error {
	return s.settings.SetTerminalFontSize(ctx, px)
}

func (s *SettingsService) SetTerminalFontFamily(ctx context.Context, family string) error {
	return s.settings.SetTerminalFontFamily(ctx, family)
}

func (s *SettingsService) SetTerminalFontWeights(ctx context.Context, weight, weightBold int) error {
	return s.settings.SetTerminalFontWeights(ctx, weight, weightBold)
}

func (s *SettingsService) SetTerminalLineHeight(ctx context.Context, lineHeight float64) error {
	return s.settings.SetTerminalLineHeight(ctx, lineHeight)
}

func (s *SettingsService) SetTerminalLetterSpacing(ctx context.Context, spacing int) error {
	return s.settings.SetTerminalLetterSpacing(ctx, spacing)
}

func (s *SettingsService) SetTerminalShowWindows(ctx context.Context, show bool) error {
	return s.settings.SetTerminalShowWindows(ctx, show)
}

func (s *SettingsService) SetTerminalShowStatusBar(ctx context.Context, show bool) error {
	return s.settings.SetTerminalShowStatusBar(ctx, show)
}

func (s *SettingsService) SetTerminalPoolSize(ctx context.Context, size int) error {
	return s.settings.SetTerminalPoolSize(ctx, size)
}

func (s *SettingsService) SetCanvasFontSize(ctx context.Context, size string) error {
	return s.settings.SetCanvasFontSize(ctx, size)
}

func (s *SettingsService) SetCanvasLineSpacing(ctx context.Context, spacing string) error {
	return s.settings.SetCanvasLineSpacing(ctx, spacing)
}

func (s *SettingsService) SetCanvasPageWidth(ctx context.Context, width string) error {
	return s.settings.SetCanvasPageWidth(ctx, width)
}

func (s *SettingsService) NotificationSettings(ctx context.Context) (NotificationSettings, error) {
	current, err := s.settings.Notifications(ctx)
	if err != nil {
		return NotificationSettings{}, err
	}
	return NotificationSettings{
		NotificationsEnabled: current.Enabled,
		Delivery:             current.Delivery,
		NotificationSound:    current.Sound,
	}, nil
}

func (s *SettingsService) SetNotificationSettings(ctx context.Context, in NotificationSettings) error {
	return s.settings.SetNotifications(ctx, app.NotificationSettings{
		Enabled:  in.NotificationsEnabled,
		Delivery: in.Delivery,
		Sound:    in.NotificationSound,
	})
}

// OnboardingSettings is whether first run has been walked to its end.
type OnboardingSettings struct {
	Completed bool `json:"completed"`
}

func (s *SettingsService) OnboardingSettings(ctx context.Context) (OnboardingSettings, error) {
	completed, err := s.settings.OnboardingCompleted(ctx)
	if err != nil {
		return OnboardingSettings{}, err
	}
	return OnboardingSettings{Completed: completed}, nil
}

func (s *SettingsService) SetOnboardingCompleted(ctx context.Context) error {
	return s.settings.SetOnboardingCompleted(ctx)
}

// EditorChoice is one editor the selector offers: its CLI command, display
// title, and whether the command resolves on the subprocess PATH right now.
type EditorChoice struct {
	Command string `json:"command"`
	Title   string `json:"title"`
	Found   bool   `json:"found"`
}

// EditorSettings is the configured "open in editor" command plus the detected
// choices the selector offers. Command is empty when none is configured; it
// may name a command outside Choices when settings.yaml was authored by hand.
// Title is Command's display name, so the frontend does not reproduce the
// command→title catalogue. Empty exactly when Command is, and the command
// itself for one outside the catalogue.
type EditorSettings struct {
	Command string         `json:"command"`
	Title   string         `json:"title"`
	Choices []EditorChoice `json:"choices"`
}

func (s *SettingsService) EditorSettings(ctx context.Context) (EditorSettings, error) {
	command, err := s.settings.Editor(ctx)
	if err != nil {
		return EditorSettings{}, err
	}
	detected := s.settings.EditorChoices(ctx)
	choices := make([]EditorChoice, 0, len(detected))
	for _, c := range detected {
		choices = append(choices, EditorChoice{Command: c.Command, Title: c.Title, Found: c.Found})
	}
	return EditorSettings{Command: command, Title: s.settings.EditorTitle(command), Choices: choices}, nil
}

// SetEditor persists the editor command; empty clears it.
func (s *SettingsService) SetEditor(ctx context.Context, command string) error {
	return s.settings.SetEditor(ctx, command)
}

func (s *SettingsService) GithubSettings(ctx context.Context) (GithubSettings, error) {
	current, err := s.settings.Github(ctx)
	if err != nil {
		return GithubSettings{}, err
	}
	return GithubSettings{
		PollIntervalSeconds:    int(current.PollInterval / time.Second),
		MinPollIntervalSeconds: int(current.MinPollInterval / time.Second),
	}, nil
}

// SetGithubSettings converts the wire's seconds to a duration and hands it to
// the core, which owns the floor and the live apply. A negative or absurd
// value is caught here because it cannot be represented as a duration at all.
func (s *SettingsService) SetGithubSettings(ctx context.Context, in GithubSettings) error {
	if in.PollIntervalSeconds < 0 {
		return app.Errorf(app.KindInvalid, "poll interval must not be negative")
	}
	if uint64(in.PollIntervalSeconds) > uint64(time.Duration(1<<63-1)/time.Second) {
		return app.Errorf(app.KindInvalid, "poll interval is too large")
	}
	return s.settings.SetGithub(ctx, app.GithubSettings{
		PollInterval: time.Duration(in.PollIntervalSeconds) * time.Second,
	})
}
