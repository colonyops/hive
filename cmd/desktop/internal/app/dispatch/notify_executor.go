package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/observe"
)

const (
	// NotifyMaxAge bounds how stale a queued notification may be when the
	// worker gets to it. A notification is a live interrupt: one enqueued
	// while the app was closed, or stranded behind a long backlog, is no
	// longer worth surfacing by the time it would fire. Past this age the
	// command completes without delivering — the second line of defense
	// behind replay's own guarantees (see the package-level note on
	// NotifyExecutor).
	NotifyMaxAge = 10 * time.Minute

	// notifyRenderedTitleMax and notifyRenderedBodyMax bound what a template
	// may render to. The config caps the template's own length, but a
	// template can expand a payload field of any size; the OS truncates the
	// banner anyway, so this only stops a runaway payload from being handed
	// to the native layer.
	notifyRenderedTitleMax = 200
	notifyRenderedBodyMax  = 1000
)

// SystemNotification is a rendered notification, ready for the OS.
type SystemNotification struct {
	Title    string
	Body     string
	Severity string
	Sound    bool
	// InApp asks the delivery adapter to surface this inside Hive rather than
	// as an OS banner. Copied from the policy; see NotificationPolicy.InApp.
	InApp bool
	// Data rides along to the native notification and comes back when the
	// user clicks it, which is how a banner knows which item to reveal.
	Data map[string]any
}

// SystemNotifier delivers a notification through the operating system.
type SystemNotifier interface {
	Notify(ctx context.Context, n SystemNotification) error
}

// NotificationPolicy is the app-level notification configuration one
// delivery must respect.
type NotificationPolicy struct {
	// Allowed is the kill switch. False means no flow may notify, whatever
	// its nodes say.
	Allowed bool
	// Sound is the app-level sound preference. A node can silence itself,
	// but it cannot make a notification audible once this is off.
	Sound bool
	// InApp routes this delivery inside Hive instead of to an OS banner. The
	// gate resolves it from the user's delivery preference and, for the
	// automatic mode, the window's current focus — so this package needs no
	// vocabulary for either.
	InApp bool
}

// NotificationGate resolves the app-level notification policy. It is read
// live on every delivery (Settings -> Notifications), so a toggle applies to
// the very next notification: a flow node must never be able to notify past
// "notifications off".
type NotificationGate interface {
	NotificationPolicy() NotificationPolicy
}

type InboxItemLocator interface {
	IDByExternalID(ctx context.Context, profileID, sourceKind, sourceScope, externalID string) (int64, error)
}

// NotifyExecutor delivers a notify terminal's queued command as a native
// notification, rendering the node's title/body templates over the
// triggering item.
//
// It never fires for historical events. Three independent things ensure
// that: a replay recomputes the graph without committing its outputs (only
// feed claims reach the backend), a snapshot-tagged message is dropped
// before it can reach a non-feed terminal, and — for anything that slips
// past both, such as a command queued before a restart — NotifyMaxAge makes
// a stale command a no-op rather than an interrupt.
type NotifyExecutor struct {
	notifier SystemNotifier
	gate     NotificationGate
	items    InboxItemLocator
	logger   zerolog.Logger
	now      func() time.Time

	mu sync.Mutex
	// quietUntil maps a (node, item) pair to the moment its cooldown window
	// ends. Entries carry their own deadline because cooldowns are per-node:
	// pruning by any single window would evict another node's still-live
	// entry.
	quietUntil map[string]time.Time
}

// NewNotifyExecutor builds a NotifyExecutor. A nil notifier leaves the
// executor unavailable rather than acknowledging a notification it never
// sent; a nil gate means "no app-level kill switch" (tests); a nil locator
// means a delivered notification carries no item link, so a click only
// raises the window.
func NewNotifyExecutor(notifier SystemNotifier, gate NotificationGate, items InboxItemLocator, logger zerolog.Logger) *NotifyExecutor {
	return &NotifyExecutor{
		notifier:   notifier,
		gate:       gate,
		items:      items,
		logger:     logger,
		now:        time.Now,
		quietUntil: map[string]time.Time{},
	}
}

func (e *NotifyExecutor) Execute(ctx context.Context, action actions.Action, data OutputData, _ ActionInvocationInput) (ExecutionResult, error) {
	cfg, ok := action.Config.(*NotifyActionConfig)
	if !ok {
		return ExecutionResult{}, fmt.Errorf("notify executor: action %q has config type %T", action.ID, action.Config)
	}
	if e.notifier == nil {
		return ExecutionResult{}, fmt.Errorf("notify executor: no system notifier configured")
	}

	var cmd models.NotifyCommand
	if err := json.Unmarshal(data.Raw, &cmd); err != nil {
		return ExecutionResult{}, fmt.Errorf("notify: decode command payload: %w", err)
	}

	now := e.now()
	// Suppressions all complete the command successfully: the notification
	// was considered and deliberately not shown, which is not a failure to
	// retry. ExecutionResult.Attempted stays false so the worker records no
	// activity for a notification the user never saw.
	if data.CreatedAt > 0 && now.Sub(time.UnixMilli(data.CreatedAt)) > NotifyMaxAge {
		e.logger.Debug().Ctx(ctx).Str("action_id", action.ID).Msg("notify: skipped stale notification")
		return ExecutionResult{}, nil
	}
	policy := NotificationPolicy{Allowed: true, Sound: true}
	if e.gate != nil {
		policy = e.gate.NotificationPolicy()
	}
	if !policy.Allowed {
		e.logger.Debug().Ctx(ctx).Str("action_id", action.ID).Msg("notify: suppressed by notification settings")
		return ExecutionResult{}, nil
	}
	if cfg.Cooldown > 0 && e.withinCooldown(action.ID, cmd.ExternalID, now) {
		e.logger.Debug().Ctx(ctx).Str("action_id", action.ID).Str("item", cmd.ExternalID).Msg("notify: suppressed within cooldown")
		return ExecutionResult{}, nil
	}
	// Templates render over the item exactly as an action's do, so
	// `{{ .Payload.title }}` means the same thing in a notify node as in a
	// launch-session action.
	item := OutputData{
		Key:       cmd.ExternalID,
		Raw:       cmd.Item,
		CommandID: data.CommandID,
		CreatedAt: data.CreatedAt,
		IsRerun:   data.IsRerun,
	}
	if len(cmd.Item) > 0 {
		if err := json.Unmarshal(cmd.Item, &item.Payload); err != nil {
			return ExecutionResult{}, fmt.Errorf("notify: decode item payload: %w", err)
		}
	}

	renderer := tmpl.New(tmpl.Config{})
	title, err := renderer.Render(cfg.Title, item)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("notify: title: %w", err)
	}
	title = boundRendered(strings.TrimSpace(title), notifyRenderedTitleMax)
	if title == "" {
		return ExecutionResult{}, fmt.Errorf("notify: title rendered blank")
	}
	body, err := renderer.Render(cfg.Body, item)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("notify: body: %w", err)
	}
	body = boundRendered(strings.TrimSpace(body), notifyRenderedBodyMax)

	if err := e.deliver(ctx, SystemNotification{
		Title:    title,
		Body:     body,
		Severity: cfg.Severity,
		Sound:    cfg.Sound && policy.Sound,
		InApp:    policy.InApp,
		Data:     e.clickData(ctx, cmd),
	}); err != nil {
		return ExecutionResult{Attempted: true}, fmt.Errorf("notify: %w", err)
	}
	// Recorded only now: a delivery that never happened — a template error,
	// a refusal from the OS — must not start a window that silences the
	// worker's own retry of the same command.
	if cfg.Cooldown > 0 {
		e.recordFired(action.ID, cmd.ExternalID, cfg.Cooldown, now)
	}
	e.logger.Info().Ctx(ctx).Str("action_id", action.ID).Msg("notify: notification delivered")
	return ExecutionResult{Attempted: true}, nil
}

// The wait is the OS's, and an in-app delivery is a different one: a banner
// goes to the notification centre while the app renders its own.
func (e *NotifyExecutor) deliver(ctx context.Context, n SystemNotification) (err error) {
	ctx, span := observe.StartConditionalSpan(ctx, tracer, "dispatch.notify", trace.WithAttributes(
		attribute.String(attrSeverity, n.Severity),
		attribute.Bool(attrInApp, n.InApp),
	))
	defer observe.End(span, &err)

	return e.notifier.Notify(ctx, n)
}

// clickData resolves what a click on the delivered banner should reveal. An
// unresolvable item is not an error — the notification still fires, and
// clicking it just raises the window.
func (e *NotifyExecutor) clickData(ctx context.Context, cmd models.NotifyCommand) map[string]any {
	data := map[string]any{"profileId": cmd.ProfileID}
	if e.items == nil {
		return data
	}
	itemID, err := e.items.IDByExternalID(ctx, cmd.ProfileID, cmd.SourceKind, cmd.SourceScope, cmd.ExternalID)
	if err != nil {
		e.logger.Warn().Ctx(ctx).Err(err).Str("profile_id", cmd.ProfileID).Msg("notify: resolving notification item failed")
		return data
	}
	if itemID != 0 {
		data["itemId"] = itemID
	}
	return data
}

// withinCooldown reports whether this node already notified about this item
// recently enough that doing so again would just be a repeat interrupt. The
// window is the node's own resolved cooldown, so each notify node sets its
// per-item delivery floor. A command with no item identity is never coalesced
// — there is nothing to coalesce it against.
//
// The window is deliberately in-memory: it bounds how often the user is
// interrupted, which is a property of this running session, not of the
// durable queue (that is what the command's dedup key is for). A restart
// starting everyone's window fresh is the right behavior.
func (e *NotifyExecutor) withinCooldown(actionID, externalID string, now time.Time) bool {
	if externalID == "" {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	deadline, ok := e.quietUntil[cooldownKey(actionID, externalID)]
	return ok && now.Before(deadline)
}

// recordFired starts this (node, item) pair's cooldown window.
func (e *NotifyExecutor) recordFired(actionID, externalID string, cooldown time.Duration, now time.Time) {
	if externalID == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	// The map holds one entry per (node, item) pair, and an entry stops
	// mattering once its own deadline passes — drop those while we hold the
	// lock rather than growing forever across a long-running session.
	for k, deadline := range e.quietUntil {
		if !now.Before(deadline) {
			delete(e.quietUntil, k)
		}
	}
	e.quietUntil[cooldownKey(actionID, externalID)] = now.Add(cooldown)
}

func cooldownKey(actionID, externalID string) string {
	return actionID + "\x00" + externalID
}

// boundRendered caps what a rendered template hands to the native layer,
// dropping the partial rune a byte-wise cut can leave behind.
func boundRendered(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.ToValidUTF8(s[:max], "") + "…"
}
