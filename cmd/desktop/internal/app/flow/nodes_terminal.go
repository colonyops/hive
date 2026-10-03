package flow

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/colonyops/hive/cmd/desktop/internal/app/icons"
	"github.com/colonyops/hive/cmd/desktop/internal/app/slug"
)

// feedDescriptionMaxLen caps a feed's hover description. It is generous enough
// for a sentence or two of context (useful for LLM-generated feeds) while
// keeping the persisted YAML and the sidebar tooltip bounded.
const feedDescriptionMaxLen = 500

// FeedConfig is a feed node: 1 input, 0 outputs (terminal). The node *is* the
// feed — its identity is the node id (flow-qualified as "<flowId>/<nodeId>"
// for membership claims). Icon and Description are purely cosmetic
// sidebar presentation: the icon shown in the tree and the tooltip surfaced
// on hover (handy context for LLM-generated feeds). Both are optional.
type FeedConfig struct {
	Icon        string `json:"icon,omitempty"        yaml:"icon,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

func (c *FeedConfig) Inputs() int  { return 1 }
func (c *FeedConfig) Outputs() int { return 0 }

func (c *FeedConfig) Validate(Refs) error {
	if !icons.ValidFeed(c.Icon) {
		return fmt.Errorf("icon: %q is not a supported feed icon", c.Icon)
	}
	if utf8.RuneCountInString(c.Description) > feedDescriptionMaxLen {
		return fmt.Errorf("description: must be at most %d characters", feedDescriptionMaxLen)
	}
	return nil
}

// ActionConfig is an action node: 1 input, 0 outputs (terminal). It enqueues
// an output_command against the referenced desktop actions.yml action id.
type ActionConfig struct {
	Action string `json:"action" yaml:"action"`
}

func (c *ActionConfig) Inputs() int  { return 1 }
func (c *ActionConfig) Outputs() int { return 0 }

func (c *ActionConfig) Validate(refs Refs) error {
	if c.Action == "" {
		return fmt.Errorf("action: action is required")
	}
	if !slug.Valid(c.Action) {
		return fmt.Errorf("action: action %q is not a valid id", c.Action)
	}
	if !refsResolveAction(refs, c.Action) {
		return fmt.Errorf("action: action %q: unresolved reference", c.Action)
	}
	if !refsActionHeadlessCapable(refs, c.Action) {
		return fmt.Errorf("action: action %q cannot run in a flow: it is a detail-pane action needing interactive input or a clipboard target", c.Action)
	}
	return nil
}

// notifySeverities is the closed set a notify node may declare. It is the
// same vocabulary the app-level notification path already speaks
// (cmd/desktop/internal/adapter/wailsui's Input.Severity), so a flow node and a built-in
// notification map to the same native interruption level rather than
// inventing a second urgency scale.
var notifySeverities = map[string]bool{
	"info":    true,
	"success": true,
	"warning": true,
	"error":   true,
}

// NotifySeverityDefault is the severity a notify node uses when it declares
// none: an ordinary, non-interrupting banner.
const NotifySeverityDefault = "info"

// NotifyCooldownDefault is the per-item delivery floor a notify node uses
// when it declares no cooldown of its own: once a node interrupts about an
// item, it stays quiet about that same item for this long.
const NotifyCooldownDefault = 5 * time.Minute

// notifyTitleMaxLen and notifyBodyMaxLen cap the *templates*, not the
// rendered result — enough room for a sentence of context each while keeping
// the persisted YAML bounded. The executor separately bounds what a template
// actually renders to.
const (
	notifyTitleMaxLen = 200
	notifyBodyMaxLen  = 1000
)

// NotifyConfig is a notify node: 1 input, 0 outputs (terminal). Every
// arriving message enqueues an output_command that the notify executor
// delivers as a native OS notification, with Title/Body rendered as Go
// text/templates over the message (the same templating story as an action's
// prompt_template — see cmd/desktop/internal/app/actions/launch_session.go).
//
// The node cannot override the app's notification settings: the executor
// checks the global kill switch before every delivery, so "notifications
// off" always wins over any flow.
type NotifyConfig struct {
	// Title renders the notification's headline. Required — a native
	// notification without a title is rejected by the OS layer.
	Title string `json:"title" yaml:"title"`
	// Body renders the notification's message. Optional.
	Body string `json:"body,omitempty" yaml:"body,omitempty"`
	// Severity selects the native interruption level (see notifySeverities).
	// Empty means NotifySeverityDefault.
	Severity string `json:"severity,omitempty" yaml:"severity,omitempty"`
	// Sound is a pointer so an absent key ("use the default", which is sound
	// on) is distinguishable from an explicit `sound: false`. It can only
	// silence a notification — the global notification-sound setting still
	// wins when it is off. Resolve through SoundOrDefault.
	Sound *bool `json:"sound,omitempty" yaml:"sound,omitempty"`
	// CooldownSeconds is a per-item delivery floor: once this node interrupts
	// about an item, it stays quiet about that same item for this long. A
	// pointer so an absent key ("use NotifyCooldownDefault") is distinct from
	// an explicit 0 ("no cooldown; every accepted message may interrupt").
	// This is a delivery floor, not dedup — deciding *whether* an item is
	// worth notifying is upstream's job. Resolve through CooldownOrDefault.
	CooldownSeconds *int `json:"cooldownSeconds,omitempty" yaml:"cooldownSeconds,omitempty"`
}

func (c *NotifyConfig) Inputs() int  { return 1 }
func (c *NotifyConfig) Outputs() int { return 0 }

// NotifyDeclaration satisfies dispatch's notify-raiser capability: a notify
// node always has content to raise — it notifies for whatever is routed to
// it, which is the author's explicit choice in authoring the node at all.
func (c *NotifyConfig) NotifyDeclaration() (cfg *NotifyConfig, ok bool) {
	return c, true
}

// SeverityOrDefault resolves Severity, defaulting to NotifySeverityDefault
// when the node declares none.
func (c *NotifyConfig) SeverityOrDefault() string {
	if c.Severity == "" {
		return NotifySeverityDefault
	}
	return c.Severity
}

// SoundOrDefault resolves Sound, defaulting to true (play the notification
// sound) when the key is absent.
func (c *NotifyConfig) SoundOrDefault() bool {
	if c.Sound == nil {
		return true
	}
	return *c.Sound
}

// CooldownOrDefault resolves CooldownSeconds, defaulting to
// NotifyCooldownDefault when the key is absent. An explicit 0 disables the
// cooldown entirely.
func (c *NotifyConfig) CooldownOrDefault() time.Duration {
	if c.CooldownSeconds == nil {
		return NotifyCooldownDefault
	}
	return time.Duration(*c.CooldownSeconds) * time.Second
}

func (c *NotifyConfig) Validate(Refs) error {
	if strings.TrimSpace(c.Title) == "" {
		return fmt.Errorf("title: title is required")
	}
	if utf8.RuneCountInString(c.Title) > notifyTitleMaxLen {
		return fmt.Errorf("title: must be at most %d characters", notifyTitleMaxLen)
	}
	if utf8.RuneCountInString(c.Body) > notifyBodyMaxLen {
		return fmt.Errorf("body: must be at most %d characters", notifyBodyMaxLen)
	}
	if c.Severity != "" && !notifySeverities[c.Severity] {
		return fmt.Errorf("severity: %q is not a supported severity (info, success, warning, error)", c.Severity)
	}
	if c.CooldownSeconds != nil && *c.CooldownSeconds < 0 {
		return fmt.Errorf("cooldownSeconds: must not be negative")
	}
	return nil
}
