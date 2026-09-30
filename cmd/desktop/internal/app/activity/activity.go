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

import (
	"fmt"
	"maps"
)

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
	// Metadata is opaque to storage. Keys under "link." are reserved for the
	// Activity view's generic destinations; other keys carry event-specific
	// data such as a retryable session draft.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Link metadata uses link.url for an external destination and the
// link.item.* group for one internal inbox reference.
const (
	MetadataLinkURL             = "link.url"
	MetadataLinkItemProfileID   = "link.item.profileId"
	MetadataLinkItemSourceKind  = "link.item.sourceKind"
	MetadataLinkItemSourceScope = "link.item.sourceScope"
	MetadataLinkItemExternalID  = "link.item.externalId"
)

// Link declares the destinations associated with an activity event. URL opens
// outside the app. Item identifies an inbox row to select inside the app.
type Link struct {
	URL  string
	Item *ItemLink
}

// ItemLink is the stable source identity of an inbox item. All fields travel
// together because an external id can be repeated across profiles and sources.
type ItemLink struct {
	ProfileID   string
	SourceKind  string
	SourceScope string
	ExternalID  string
}

// WithLink adds the generic link metadata the Activity view understands.
func (e Event) WithLink(link Link) Event {
	if link.URL == "" && link.Item == nil {
		return e
	}
	metadata := make(map[string]string, len(e.Metadata)+5)
	maps.Copy(metadata, e.Metadata)
	if link.URL != "" {
		metadata[MetadataLinkURL] = link.URL
	}
	if link.Item != nil {
		metadata[MetadataLinkItemProfileID] = link.Item.ProfileID
		metadata[MetadataLinkItemSourceKind] = link.Item.SourceKind
		metadata[MetadataLinkItemSourceScope] = link.Item.SourceScope
		metadata[MetadataLinkItemExternalID] = link.Item.ExternalID
	}
	e.Metadata = metadata
	return e
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

// SessionCreateFailed records a session the app accepted and could not create.
// Unlike ActionFailed there is nothing to show afterwards, so retry carries
// the submitted form and makes the row the way back to it
// (ADR a-failed-session-creation-is-a-retryable-draft).
func SessionCreateFailed(name, repo, step, reason string, retry map[string]string) Event {
	return Event{
		Category: CategorySession,
		Severity: SeverityError,
		Title:    fmt.Sprintf("Could not create session %s", name),
		Body:     joinMeta(repo, step, reason),
		Source:   name,
		Metadata: retry,
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

// status is schedule.Status's string form rather than the type: the audit log
// is a sink every subsystem writes to and takes no dependency on any of them,
// so an unrecognized status records as plain info instead of failing.
func ScheduleRun(name, workspace, status, detail string) Event {
	severity, outcome := SeverityInfo, "ran"
	switch status {
	case "launched":
		severity, outcome = SeveritySuccess, "launched"
	case "skipped":
		severity, outcome = SeverityWarning, "skipped"
	case "failed":
		severity, outcome = SeverityError, "failed"
	}
	return Event{
		Category: CategoryAutoAction,
		Severity: severity,
		Title:    fmt.Sprintf("Scheduled chat %q %s", name, outcome),
		Body:     joinMeta(workspace, detail),
		Source:   workspace,
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

// FlowRuntimeFailed records a flow the engine could not put into service. The
// last known-good version of that flow keeps running, which is exactly why
// this has to be recorded: without it the failure is invisible and the app
// silently executes an older graph than the one on disk.
func FlowRuntimeFailed(flowID string, err error) Event {
	return Event{
		Category: CategoryConfig,
		Severity: SeverityError,
		Title:    fmt.Sprintf("Flow %s could not be deployed", flowID),
		Body:     err.Error(),
		Source:   flowID,
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
