// Package activity is the desktop app's user-facing audit log. Any backend
// subsystem — a source refresh, a session launch, an action run, a config
// reload — records a well-formed [Event] through a [Recorder], and the frontend
// records its own via the ActivityService. Events persist in the pipeline
// SQLite store and surface, newest first, in the Activity view.
//
// The ergonomic constructors ([RefreshFailed], [SessionCreated], [AutoAction],
// …) are the "mixin" surface: an emit site names what happened and passes the few
// facts it has, and the constructor assembles the category, severity, and human
// copy. New event shapes are added as new constructors, not new call-site
// formatting.
package activity

import "fmt"

// Category is the semantic bucket of an event. It drives the Activity view's
// filter pills and, for non-error events, the row's icon and accent color.
//
// ENUM(refresh, session, auto_action, action, config, system)
type Category string

// Severity is the emphasis of an event, independent of its category: a refresh
// that failed is Category=refresh, Severity=error. It selects error/auto row
// styling and bridges to the toast system's matching severities.
//
// ENUM(info, success, warning, error, auto)
type Severity string

// Event is one activity-log entry. ID and CreatedAt are assigned by the store
// on append; constructors and callers leave them zero. CreatedAt is unix
// milliseconds (see the activity_event migration for why milliseconds).
type Event struct {
	ID        int64    `json:"id"`
	CreatedAt int64    `json:"createdAt"`
	Category  Category `json:"category"`
	Severity  Severity `json:"severity"`
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Source    string   `json:"source,omitempty"`
	// Metadata is opaque to storage and reserved for later enrichment (links,
	// ids, counts). No emit site populates it yet; it round-trips through the
	// reserved metadata column so a future write path needs no schema change.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RefreshFailed records a source refresh that errored, capturing the reason
// (an error string or stderr excerpt).
func RefreshFailed(source, reason string) Event {
	return Event{
		Category: CategoryRefresh,
		Severity: SeverityError,
		Title:    fmt.Sprintf("Refresh failed for %s", source),
		Body:     reason,
		Source:   source,
	}
}

// SessionCreated records a hive session created by an action. agent and repo
// are optional and folded into the metadata line when present.
func SessionCreated(name, agent, repo string) Event {
	return Event{
		Category: CategorySession,
		Severity: SeveritySuccess,
		Title:    fmt.Sprintf("Created session %s", name),
		Body:     joinMeta(agent, repo),
		Source:   name,
	}
}

// AutoAction records an automatic action applied without confirmation. rule is
// the action/rule id and target the item it acted on.
func AutoAction(label, rule, target string) Event {
	title := "Auto-action"
	if label != "" {
		title = fmt.Sprintf("Auto-action · %s", label)
	}
	body := "no confirmation required"
	if target != "" {
		body = fmt.Sprintf("%s · %s", target, body)
	}
	if rule != "" {
		body = fmt.Sprintf("rule %s · %s", rule, body)
	}
	return Event{
		Category: CategoryAutoAction,
		Severity: SeverityAuto,
		Title:    title,
		Body:     body,
		Source:   rule,
	}
}

// ActionRun records a manually confirmed action that succeeded. detail is an
// optional command/summary line.
func ActionRun(label, detail string) Event {
	return Event{
		Category: CategoryAction,
		Severity: SeveritySuccess,
		Title:    fmt.Sprintf("Ran %s", label),
		Body:     detail,
		Source:   label,
	}
}

// ActionFailed records an action (auto or manual) that failed, capturing the
// error reason.
func ActionFailed(label, reason string) Event {
	return Event{
		Category: CategoryAction,
		Severity: SeverityError,
		Title:    fmt.Sprintf("%s failed", label),
		Body:     reason,
		Source:   label,
	}
}

// ConfigReloaded records a config file reload, reporting the resulting action
// count.
func ConfigReloaded(file string, actions int) Event {
	return Event{
		Category: CategoryConfig,
		Severity: SeverityInfo,
		Title:    fmt.Sprintf("Reloaded %s", file),
		Body:     fmt.Sprintf("%d actions", actions),
		Source:   file,
	}
}

// RecordInput is the frontend-facing shape for recording an event over the
// Wails boundary. Category and severity are plain strings resolved (case
// -insensitively) by [RecordInput.Event]; empty values default to system/info.
type RecordInput struct {
	Category string            `json:"category"`
	Severity string            `json:"severity"`
	Title    string            `json:"title"`
	Body     string            `json:"body"`
	Source   string            `json:"source"`
	Metadata map[string]string `json:"metadata"`
}

// Event validates the input and converts it to a domain [Event].
func (in RecordInput) Event() (Event, error) {
	if in.Title == "" {
		return Event{}, fmt.Errorf("activity event requires a title")
	}
	category := CategorySystem
	if in.Category != "" {
		parsed, err := ParseCategory(in.Category)
		if err != nil {
			return Event{}, fmt.Errorf("invalid activity category %q: %w", in.Category, err)
		}
		category = parsed
	}
	severity := SeverityInfo
	if in.Severity != "" {
		parsed, err := ParseSeverity(in.Severity)
		if err != nil {
			return Event{}, fmt.Errorf("invalid activity severity %q: %w", in.Severity, err)
		}
		severity = parsed
	}
	return Event{
		Category: category,
		Severity: severity,
		Title:    in.Title,
		Body:     in.Body,
		Source:   in.Source,
		Metadata: in.Metadata,
	}, nil
}

func joinMeta(parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out == "" {
			out = p
			continue
		}
		out += " · " + p
	}
	return out
}
