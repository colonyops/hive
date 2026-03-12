package config

import (
	"maps"
	"time"

	"github.com/hay-kot/criterio"
)

// ViewsConfig holds per-view configuration (keybindings, layout, behavior).
type ViewsConfig struct {
	Global   GlobalViewConfig   `json:"global"   yaml:"global"`
	Sessions SessionsViewConfig `json:"sessions" yaml:"sessions"`
	Tasks    TasksViewConfig    `json:"tasks"    yaml:"tasks"`
	Messages MessagesViewConfig `json:"messages" yaml:"messages"`
	Review   ReviewViewConfig   `json:"review"   yaml:"review"`
}

// GlobalViewConfig holds configuration for global (cross-view) keybindings.
type GlobalViewConfig struct {
	Keybindings map[string]Keybinding `json:"keybindings" yaml:"keybindings"`
}

// SessionsViewConfig holds configuration for the sessions view.
type SessionsViewConfig struct {
	Keybindings     map[string]Keybinding `json:"keybindings"      yaml:"keybindings"`
	SplitRatio      int                   `json:"split_ratio"      yaml:"split_ratio"`
	RefreshInterval time.Duration         `json:"refresh_interval" yaml:"refresh_interval"`
	PreviewEnabled  bool                  `json:"preview_enabled"  yaml:"preview_enabled"`
	PreviewTitle    string                `json:"preview_title"    yaml:"preview_title"`
	PreviewStatus   string                `json:"preview_status"   yaml:"preview_status"`
	GroupBy         string                `json:"group_by"         yaml:"group_by"`
}

// SplitRatioOrDefault returns the configured split ratio, or the given default if unset or invalid.
func (s SessionsViewConfig) SplitRatioOrDefault(defaultPct int) int {
	if s.SplitRatio < 1 || s.SplitRatio > 80 {
		return defaultPct
	}
	return s.SplitRatio
}

// TasksViewConfig holds configuration for the tasks view.
type TasksViewConfig struct {
	Keybindings map[string]Keybinding `json:"keybindings" yaml:"keybindings"`
	SplitRatio  int                   `json:"split_ratio" yaml:"split_ratio"`
}

// SplitRatioOrDefault returns the configured split ratio, or the given default if unset or invalid.
func (t TasksViewConfig) SplitRatioOrDefault(defaultPct int) int {
	if t.SplitRatio < 1 || t.SplitRatio > 80 {
		return defaultPct
	}
	return t.SplitRatio
}

// ReviewViewConfig holds configuration for the review/docs view.
type ReviewViewConfig struct {
	Keybindings map[string]Keybinding `json:"keybindings" yaml:"keybindings"`
	SplitRatio  int                   `json:"split_ratio" yaml:"split_ratio"`
}

// SplitRatioOrDefault returns the configured split ratio, or the given default if unset or invalid.
func (r ReviewViewConfig) SplitRatioOrDefault(defaultPct int) int {
	if r.SplitRatio < 1 || r.SplitRatio > 80 {
		return defaultPct
	}
	return r.SplitRatio
}

// MessagesViewConfig holds configuration for the messages view.
type MessagesViewConfig struct {
	Keybindings map[string]Keybinding `json:"keybindings" yaml:"keybindings"`
	SplitRatio  int                   `json:"split_ratio" yaml:"split_ratio"`
}

// SplitRatioOrDefault returns the configured split ratio, or the given default if unset or invalid.
func (m MessagesViewConfig) SplitRatioOrDefault(defaultPct int) int {
	if m.SplitRatio < 1 || m.SplitRatio > 80 {
		return defaultPct
	}
	return m.SplitRatio
}

// defaultViewsConfig provides built-in per-view keybindings that users can override.
var defaultViewsConfig = ViewsConfig{
	Global: GlobalViewConfig{
		Keybindings: map[string]Keybinding{},
	},
	Sessions: SessionsViewConfig{
		Keybindings: map[string]Keybinding{
			"r":      {Cmd: "Recycle"},
			"d":      {Cmd: "Delete"},
			"n":      {Cmd: "NewSession"},
			"enter":  {Cmd: "TmuxOpen"},
			"ctrl+d": {Cmd: "TmuxKill"},
			"A":      {Cmd: "AgentSend"},
			"p":      {Cmd: "TmuxPopUp"},
			"R":      {Cmd: "RenameSession"},
			"ctrl+g": {Cmd: "GroupSet"},
			"J":      {Cmd: "NextActive"},
			"K":      {Cmd: "PrevActive"},
			"t":      {Cmd: "TodoPanel"},
			"T":      {Cmd: "ViewTasks"},
		},
	},
	Tasks: TasksViewConfig{
		Keybindings: map[string]Keybinding{
			"r": {Cmd: "TasksRefresh"},
			"f": {Cmd: "TasksFilter"},
			"y": {Cmd: "TasksCopyID"},
			"v": {Cmd: "TasksTogglePreview"},
			"s": {Cmd: "TasksSelectRepo"},
			"o": {Cmd: "TasksSetOpen"},
			"i": {Cmd: "TasksSetInProgress"},
			"d": {Cmd: "TasksSetDone"},
			"x": {Cmd: "TasksSetCancelled"},
			"D": {Cmd: "TasksDelete"},
			"P": {Cmd: "TasksPrune"},
		},
	},
	Review: ReviewViewConfig{
		Keybindings: map[string]Keybinding{
			"y": {Cmd: "DocsCopyPath"},
			"Y": {Cmd: "DocsCopyRelPath"},
			"c": {Cmd: "DocsCopyContents"},
			"o": {Cmd: "DocsOpen"},
			"v": {Cmd: "DocsTogglePreview"},
			"r": {Cmd: "DocsSelectRepo"},
		},
	},
}

func mergeKeybindingMaps(defaults, user map[string]Keybinding) map[string]Keybinding {
	merged := make(map[string]Keybinding, len(defaults))
	maps.Copy(merged, defaults)
	maps.Copy(merged, user)
	return merged
}

func mergeViewsConfig(defaults, user ViewsConfig) ViewsConfig {
	return ViewsConfig{
		Global: GlobalViewConfig{
			Keybindings: mergeKeybindingMaps(defaults.Global.Keybindings, user.Global.Keybindings),
		},
		Sessions: SessionsViewConfig{
			Keybindings:     mergeKeybindingMaps(defaults.Sessions.Keybindings, user.Sessions.Keybindings),
			SplitRatio:      firstNonZero(user.Sessions.SplitRatio, defaults.Sessions.SplitRatio),
			RefreshInterval: firstNonZeroDuration(user.Sessions.RefreshInterval, defaults.Sessions.RefreshInterval),
			PreviewEnabled:  user.Sessions.PreviewEnabled || defaults.Sessions.PreviewEnabled,
			PreviewTitle:    firstNonEmpty(user.Sessions.PreviewTitle, defaults.Sessions.PreviewTitle),
			PreviewStatus:   firstNonEmpty(user.Sessions.PreviewStatus, defaults.Sessions.PreviewStatus),
			GroupBy:         firstNonEmpty(user.Sessions.GroupBy, defaults.Sessions.GroupBy),
		},
		Tasks: TasksViewConfig{
			Keybindings: mergeKeybindingMaps(defaults.Tasks.Keybindings, user.Tasks.Keybindings),
			SplitRatio:  firstNonZero(user.Tasks.SplitRatio, defaults.Tasks.SplitRatio),
		},
		Messages: MessagesViewConfig{
			Keybindings: mergeKeybindingMaps(defaults.Messages.Keybindings, user.Messages.Keybindings),
			SplitRatio:  firstNonZero(user.Messages.SplitRatio, defaults.Messages.SplitRatio),
		},
		Review: ReviewViewConfig{
			Keybindings: mergeKeybindingMaps(defaults.Review.Keybindings, user.Review.Keybindings),
			SplitRatio:  firstNonZero(user.Review.SplitRatio, defaults.Review.SplitRatio),
		},
	}
}

func firstNonZero(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}

func firstNonZeroDuration(a, b time.Duration) time.Duration {
	if a != 0 {
		return a
	}
	return b
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (v *ViewsConfig) keybindingsForView(view string) map[string]Keybinding {
	switch view {
	case "sessions":
		return v.Sessions.Keybindings
	case "tasks":
		return v.Tasks.Keybindings
	case "messages":
		return v.Messages.Keybindings
	case "review":
		return v.Review.Keybindings
	case "global":
		return v.Global.Keybindings
	default:
		return nil
	}
}

func (v *ViewsConfig) flattenedForView(view string) map[string]Keybinding {
	result := make(map[string]Keybinding)
	maps.Copy(result, v.Global.Keybindings)
	if viewKBs := v.keybindingsForView(view); viewKBs != nil {
		maps.Copy(result, viewKBs)
	}
	return result
}

func validateViewKeybindingMaps(errs *criterio.FieldErrorsBuilder, views ViewsConfig) {
	validateKeybindingMap(errs, "views.global.keybindings", views.Global.Keybindings)
	validateKeybindingMap(errs, "views.sessions.keybindings", views.Sessions.Keybindings)
	validateKeybindingMap(errs, "views.tasks.keybindings", views.Tasks.Keybindings)
	validateKeybindingMap(errs, "views.messages.keybindings", views.Messages.Keybindings)
	validateKeybindingMap(errs, "views.review.keybindings", views.Review.Keybindings)
}
