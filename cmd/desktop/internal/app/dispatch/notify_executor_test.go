package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

type notifierTest struct {
	sent []SystemNotification
	err  error
}

func (n *notifierTest) Notify(_ context.Context, notification SystemNotification) error {
	n.sent = append(n.sent, notification)
	return n.err
}

type gateTest struct{ policy NotificationPolicy }

func (g gateTest) NotificationPolicy() NotificationPolicy { return g.policy }

type itemLocatorTest struct {
	id  int64
	err error
}

func (l itemLocatorTest) IDByExternalID(context.Context, string, string, string, string) (int64, error) {
	return l.id, l.err
}

func notifyAction() actions.Action {
	return actions.Action{
		ID:    models.NotifyActionID("triage/tell-me"),
		Label: "Tell me",
		Type:  ActionTypeNotify,
		Config: &NotifyActionConfig{
			Title:    "{{ .Payload.repo }} needs review",
			Body:     "{{ .Payload.title }}",
			Severity: "warning",
			Sound:    true,
			Cooldown: flow.NotifyCooldownDefault,
		},
	}
}

func notifyData(t *testing.T, cmd models.NotifyCommand) OutputData {
	t.Helper()
	raw, err := json.Marshal(cmd)
	require.NoError(t, err)
	return OutputData{Key: "occurrence-1", Raw: raw, CreatedAt: time.Now().UnixMilli(), CommandID: 7}
}

func notifyCommand() models.NotifyCommand {
	return models.NotifyCommand{
		ProfileID:   "triage",
		ExternalID:  "acme/api#12",
		SourceKind:  "github",
		SourceScope: "src",
		Item:        json.RawMessage(`{"repo":"acme/api","title":"Fix the flake"}`),
	}
}

func openGate() gateTest { return gateTest{policy: NotificationPolicy{Allowed: true, Sound: true}} }

func TestNotifyExecutor_RendersTemplatesAndLinksTheItem(t *testing.T) {
	notifier := &notifierTest{}
	executor := NewNotifyExecutor(notifier, openGate(), itemLocatorTest{id: 42}, zerolog.Nop())

	result, err := executor.Execute(t.Context(), notifyAction(), notifyData(t, notifyCommand()), ActionInvocationInput{})
	require.NoError(t, err)
	assert.True(t, result.Attempted)
	require.Len(t, notifier.sent, 1)

	sent := notifier.sent[0]
	assert.Equal(t, "acme/api needs review", sent.Title)
	assert.Equal(t, "Fix the flake", sent.Body)
	assert.Equal(t, "warning", sent.Severity)
	assert.True(t, sent.Sound)
	assert.Equal(t, map[string]any{"profileId": "triage", "itemId": int64(42)}, sent.Data)
}

// An item the inbox no longer holds still notifies — the click just has
// nothing to select.
func TestNotifyExecutor_DeliversWithoutAResolvableItem(t *testing.T) {
	notifier := &notifierTest{}
	executor := NewNotifyExecutor(notifier, openGate(), itemLocatorTest{err: errors.New("gone")}, zerolog.Nop())

	_, err := executor.Execute(t.Context(), notifyAction(), notifyData(t, notifyCommand()), ActionInvocationInput{})
	require.NoError(t, err)
	require.Len(t, notifier.sent, 1)
	assert.Equal(t, map[string]any{"profileId": "triage"}, notifier.sent[0].Data)
}

func TestNotifyExecutor_GlobalSettingsWinOverTheNode(t *testing.T) {
	t.Run("notifications off suppresses delivery", func(t *testing.T) {
		notifier := &notifierTest{}
		executor := NewNotifyExecutor(notifier, gateTest{}, itemLocatorTest{}, zerolog.Nop())

		result, err := executor.Execute(t.Context(), notifyAction(), notifyData(t, notifyCommand()), ActionInvocationInput{})
		require.NoError(t, err, "a suppressed notification completes rather than retrying")
		assert.False(t, result.Attempted, "a suppressed notification is not an activity event")
		assert.Empty(t, notifier.sent)
	})

	t.Run("sound off silences a node that asked for sound", func(t *testing.T) {
		notifier := &notifierTest{}
		gate := gateTest{policy: NotificationPolicy{Allowed: true}}
		executor := NewNotifyExecutor(notifier, gate, itemLocatorTest{}, zerolog.Nop())

		_, err := executor.Execute(t.Context(), notifyAction(), notifyData(t, notifyCommand()), ActionInvocationInput{})
		require.NoError(t, err)
		require.Len(t, notifier.sent, 1)
		assert.False(t, notifier.sent[0].Sound)
	})
}

func TestNotifyExecutor_SkipsStaleCommands(t *testing.T) {
	notifier := &notifierTest{}
	executor := NewNotifyExecutor(notifier, openGate(), itemLocatorTest{id: 42}, zerolog.Nop())

	data := notifyData(t, notifyCommand())
	data.CreatedAt = time.Now().Add(-NotifyMaxAge - time.Minute).UnixMilli()

	result, err := executor.Execute(t.Context(), notifyAction(), data, ActionInvocationInput{})
	require.NoError(t, err)
	assert.False(t, result.Attempted)
	assert.Empty(t, notifier.sent, "a notification queued before a restart must not arrive late")
}

func TestNotifyExecutor_CoalescesRepeatsForTheSameItem(t *testing.T) {
	notifier := &notifierTest{}
	executor := NewNotifyExecutor(notifier, openGate(), itemLocatorTest{id: 42}, zerolog.Nop())
	now := time.Now()
	executor.now = func() time.Time { return now }

	_, err := executor.Execute(t.Context(), notifyAction(), notifyData(t, notifyCommand()), ActionInvocationInput{})
	require.NoError(t, err)

	// A second, genuinely different occurrence for the same item within the
	// window is a repeat interrupt, not new information.
	result, err := executor.Execute(t.Context(), notifyAction(), notifyData(t, notifyCommand()), ActionInvocationInput{})
	require.NoError(t, err)
	assert.False(t, result.Attempted)
	assert.Len(t, notifier.sent, 1)

	// A different item is never coalesced against the first.
	other := notifyCommand()
	other.ExternalID = "acme/api#13"
	_, err = executor.Execute(t.Context(), notifyAction(), notifyData(t, other), ActionInvocationInput{})
	require.NoError(t, err)
	assert.Len(t, notifier.sent, 2)

	// Past the window the same item may interrupt again.
	now = now.Add(flow.NotifyCooldownDefault + time.Second)
	_, err = executor.Execute(t.Context(), notifyAction(), notifyData(t, notifyCommand()), ActionInvocationInput{})
	require.NoError(t, err)
	assert.Len(t, notifier.sent, 3)
}

// The cooldown window is the node's own resolved config: a short window
// suppresses inside it and delivers past it, and an explicit 0 means every
// accepted delivery may interrupt.
func TestNotifyExecutor_CooldownIsConfigurable(t *testing.T) {
	withCooldown := func(cooldown time.Duration) actions.Action {
		action := notifyAction()
		cfg, ok := action.Config.(*NotifyActionConfig)
		require.True(t, ok)
		updated := *cfg
		updated.Cooldown = cooldown
		action.Config = &updated
		return action
	}

	t.Run("suppresses within the configured window and delivers past it", func(t *testing.T) {
		notifier := &notifierTest{}
		executor := NewNotifyExecutor(notifier, openGate(), itemLocatorTest{}, zerolog.Nop())
		now := time.Now()
		executor.now = func() time.Time { return now }

		action := withCooldown(30 * time.Second)
		_, err := executor.Execute(t.Context(), action, notifyData(t, notifyCommand()), ActionInvocationInput{})
		require.NoError(t, err)

		result, err := executor.Execute(t.Context(), action, notifyData(t, notifyCommand()), ActionInvocationInput{})
		require.NoError(t, err)
		assert.False(t, result.Attempted)
		assert.Len(t, notifier.sent, 1)

		now = now.Add(31 * time.Second)
		_, err = executor.Execute(t.Context(), action, notifyData(t, notifyCommand()), ActionInvocationInput{})
		require.NoError(t, err)
		assert.Len(t, notifier.sent, 2)
	})

	t.Run("a zero cooldown never suppresses", func(t *testing.T) {
		notifier := &notifierTest{}
		executor := NewNotifyExecutor(notifier, openGate(), itemLocatorTest{}, zerolog.Nop())

		action := withCooldown(0)
		_, err := executor.Execute(t.Context(), action, notifyData(t, notifyCommand()), ActionInvocationInput{})
		require.NoError(t, err)
		_, err = executor.Execute(t.Context(), action, notifyData(t, notifyCommand()), ActionInvocationInput{})
		require.NoError(t, err)
		assert.Len(t, notifier.sent, 2)
	})

	t.Run("a shorter-cooldown node firing does not shorten another node's window", func(t *testing.T) {
		notifier := &notifierTest{}
		executor := NewNotifyExecutor(notifier, openGate(), itemLocatorTest{}, zerolog.Nop())
		now := time.Now()
		executor.now = func() time.Time { return now }

		patient := withCooldown(time.Hour)
		eager := withCooldown(30 * time.Second)
		eager.ID = models.NotifyActionID("triage/also-tell-me")

		_, err := executor.Execute(t.Context(), patient, notifyData(t, notifyCommand()), ActionInvocationInput{})
		require.NoError(t, err)

		// The eager node fires in between; its record-time pruning must not
		// evict the patient node's still-live entry.
		now = now.Add(time.Minute)
		_, err = executor.Execute(t.Context(), eager, notifyData(t, notifyCommand()), ActionInvocationInput{})
		require.NoError(t, err)
		require.Len(t, notifier.sent, 2)

		now = now.Add(time.Minute)
		result, err := executor.Execute(t.Context(), patient, notifyData(t, notifyCommand()), ActionInvocationInput{})
		require.NoError(t, err)
		assert.False(t, result.Attempted)
		assert.Len(t, notifier.sent, 2, "the patient node is still inside its hour-long window")
	})
}

// Two notify nodes fed by the same message are independent destinations, so
// one firing must not silence the other.
func TestNotifyExecutor_CooldownIsPerNode(t *testing.T) {
	notifier := &notifierTest{}
	executor := NewNotifyExecutor(notifier, openGate(), itemLocatorTest{}, zerolog.Nop())

	_, err := executor.Execute(t.Context(), notifyAction(), notifyData(t, notifyCommand()), ActionInvocationInput{})
	require.NoError(t, err)

	other := notifyAction()
	other.ID = models.NotifyActionID("triage/also-tell-me")
	_, err = executor.Execute(t.Context(), other, notifyData(t, notifyCommand()), ActionInvocationInput{})
	require.NoError(t, err)
	assert.Len(t, notifier.sent, 2)
}

func TestNotifyExecutor_RejectsBlankTitleAndBadConfig(t *testing.T) {
	notifier := &notifierTest{}
	executor := NewNotifyExecutor(notifier, openGate(), itemLocatorTest{}, zerolog.Nop())

	blank := notifyAction()
	blank.Config = &NotifyActionConfig{Title: `{{ printf "" }}`}
	_, err := executor.Execute(t.Context(), blank, notifyData(t, notifyCommand()), ActionInvocationInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blank")
	assert.Empty(t, notifier.sent)

	// A template naming a field the item does not carry fails loudly rather
	// than sending a banner with a hole in it.
	missing := notifyAction()
	missing.Config = &NotifyActionConfig{Title: "{{ .Payload.missing }}"}
	_, err = executor.Execute(t.Context(), missing, notifyData(t, notifyCommand()), ActionInvocationInput{})
	require.Error(t, err)
	assert.Empty(t, notifier.sent)

	wrongConfig := notifyAction()
	wrongConfig.Config = &actions.ShellConfig{}
	_, err = executor.Execute(t.Context(), wrongConfig, notifyData(t, notifyCommand()), ActionInvocationInput{})
	require.Error(t, err)

	_, err = NewNotifyExecutor(nil, openGate(), itemLocatorTest{}, zerolog.Nop()).
		Execute(t.Context(), notifyAction(), notifyData(t, notifyCommand()), ActionInvocationInput{})
	require.Error(t, err)
}

func TestNotifyExecutor_ReportsDeliveryFailure(t *testing.T) {
	notifier := &notifierTest{err: errors.New("permission denied")}
	executor := NewNotifyExecutor(notifier, openGate(), itemLocatorTest{}, zerolog.Nop())

	result, err := executor.Execute(t.Context(), notifyAction(), notifyData(t, notifyCommand()), ActionInvocationInput{})
	require.ErrorIs(t, err, notifier.err)
	assert.True(t, result.Attempted, "the notification was dispatched and the OS refused it")
}

// The delivery preference decides where a notification surfaces; the executor
// only carries the gate's verdict through to the adapter.
func TestNotifyExecutor_CarriesInAppDelivery(t *testing.T) {
	notifier := &notifierTest{}
	gate := gateTest{policy: NotificationPolicy{Allowed: true, Sound: true, InApp: true}}
	executor := NewNotifyExecutor(notifier, gate, itemLocatorTest{id: 42}, zerolog.Nop())

	_, err := executor.Execute(t.Context(), notifyAction(), notifyData(t, notifyCommand()), ActionInvocationInput{})
	require.NoError(t, err)
	require.Len(t, notifier.sent, 1)
	assert.True(t, notifier.sent[0].InApp)
}
