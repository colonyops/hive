package wailsui

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// NotifyInput is the frontend-facing request for a native notification.
type NotifyInput struct {
	Title    string         `json:"title"`
	Subtitle string         `json:"subtitle"`
	Body     string         `json:"body"`
	Severity string         `json:"severity"`
	Sound    bool           `json:"sound"`
	Data     map[string]any `json:"data"`
}

// notificationNotifier is the small notification API exposed to the binding.
type notificationNotifier interface {
	Notify(Input) error
	PermissionStatus() (string, error)
	RequestPermission() (bool, error)
}

// NotificationService exposes native notification delivery and authorization to
// the frontend. Window activation remains owned by main.go.
type NotificationService struct {
	notifier notificationNotifier
}

// NewNotificationService constructs the binding over a ready notifier.
func NewNotificationService(notifier notificationNotifier) *NotificationService {
	if notifier == nil {
		return NewUnavailableNotificationService(errors.New("notification service is unavailable"))
	}
	return &NotificationService{notifier: notifier}
}

// NewUnavailableNotificationService constructs a binding that reports a useful
// setup failure instead of panicking when native notifications are unavailable.
func NewUnavailableNotificationService(err error) *NotificationService {
	if err == nil {
		err = errors.New("notification service is unavailable")
	}
	return &NotificationService{notifier: unavailableNotifier{err: err}}
}

// Notify sends a native notification. NotifyInput and Input are the same
// fields either side of the wire boundary -- the former carries the JSON tags
// the binding needs, the latter does not -- so the hand-copy this used to do
// is a conversion now that both live in this package.
func (s *NotificationService) Notify(in NotifyInput) error {
	return s.notifier.Notify(Input(in))
}

// PermissionStatus reports granted, denied, or not-requested.
func (s *NotificationService) PermissionStatus() (string, error) {
	return s.notifier.PermissionStatus()
}

// RequestNotificationPermission asks the operating system for notification
// authorization and returns whether it was granted.
func (s *NotificationService) RequestNotificationPermission() (bool, error) {
	return s.notifier.RequestPermission()
}

// NotificationActivation is the notification:activated payload: which
// workspace and inbox item a clicked notification came from. ItemID is 0 when
// the notification had no item behind it (an app-level notification, or one
// whose item could not be resolved), which the frontend reads as "just raise
// the window".
type NotificationActivation struct {
	ProfileID string `json:"profileId"`
	ItemID    int64  `json:"itemId"`
}

// FlowNotifier adapts the native notifier to dispatch's SystemNotifier, so a
// flow's notify node delivers through exactly the same native path (and icon,
// and permission handling) as an app-level notification.
type FlowNotifier struct{ notifier notificationNotifier }

// NewFlowNotifier wraps a notification service's notifier as the driven port
// dispatch declares.
func NewFlowNotifier(s *NotificationService) FlowNotifier {
	return FlowNotifier{notifier: s.notifier}
}

func (n FlowNotifier) Notify(_ context.Context, in dispatch.SystemNotification) error {
	// The user asked for this one inside Hive, not as a banner. There is no
	// native call to make and no notification permission to need: the
	// in-app toast itself is not this port's job any more. events.go
	// subscribes to the core's NotificationRaised (published once this
	// Notify returns successfully, see app.go's observedNotifier) and emits
	// notification:toast from there — the frontend's behavior is unchanged,
	// only where the emission happens moved off this ad hoc call site.
	if in.InApp {
		return nil
	}
	if n.notifier == nil {
		return errors.New("native notifications unavailable")
	}
	return n.notifier.Notify(Input{
		Title:    in.Title,
		Body:     in.Body,
		Severity: in.Severity,
		Sound:    in.Sound,
		Data:     in.Data,
	})
}

// NotificationToast is a flow notification the user chose to receive inside
// Hive. It is the payload of the notification:toast event; unlike a banner it
// carries no click target, because the app is already in front of them.
type NotificationToast struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Severity string `json:"severity"`
}

// NotificationGate resolves the app-level notification policy on every
// delivery, so toggling notifications off in Settings silences flow notify
// nodes immediately rather than at the next restart. Whether notifications
// are allowed at all, and how loud, is a settings-derived decision the core
// already owns (app.SettingsService.Notifications is the same read a
// Settings screen makes) — an unreadable settings file fails closed there,
// never surfacing a notification the user may have switched off.
//
// What this gate adds is the one thing the core cannot know: where a
// notification surfaces. It holds the window's focus state because the
// automatic delivery mode means "a banner only when I'm looking elsewhere",
// and only this side of the app knows both halves.
type NotificationGate struct {
	settings *app.SettingsService
	focus    *FocusState
	logger   zerolog.Logger
}

// NewNotificationGate builds the gate over the core's settings service and
// the window's focus state.
func NewNotificationGate(settingsService *app.SettingsService, focus *FocusState, logger zerolog.Logger) NotificationGate {
	return NotificationGate{settings: settingsService, focus: focus, logger: logger}
}

func (g NotificationGate) NotificationPolicy() dispatch.NotificationPolicy {
	// dispatch.NotificationGate declares NotificationPolicy() with no context
	// parameter — it is dispatch's interface, not this package's to change —
	// so this is the one place a context is synthesized rather than threaded
	// through.
	current, err := g.settings.Notifications(context.Background())
	if err != nil {
		g.logger.Warn().Err(err).Msg("notification settings unreadable; suppressing flow notifications")
		return dispatch.NotificationPolicy{}
	}
	return dispatch.NotificationPolicy{
		Allowed: current.Enabled,
		Sound:   current.Sound,
		InApp:   g.inApp(current.Delivery),
	}
}

// inApp maps the delivery preference onto this moment: "app" always stays
// inside Hive, "system" always leaves it, and "auto" — the default — keeps a
// notification in-app only while the user is already looking at the window.
// An unknown focus state (no window yet) counts as unfocused, so a
// notification raised during startup still reaches the user.
func (g NotificationGate) inApp(delivery string) bool {
	switch delivery {
	case settings.DeliveryApp:
		return true
	case settings.DeliverySystem:
		return false
	default:
		return g.focus != nil && g.focus.Get()
	}
}

type unavailableNotifier struct {
	err error
}

func (n unavailableNotifier) Notify(Input) error {
	return fmt.Errorf("native notifications unavailable: %w", n.err)
}

func (n unavailableNotifier) PermissionStatus() (string, error) {
	return "", fmt.Errorf("native notifications unavailable: %w", n.err)
}

func (n unavailableNotifier) RequestPermission() (bool, error) {
	return false, fmt.Errorf("native notifications unavailable: %w", n.err)
}
