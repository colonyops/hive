package main

import (
	"errors"
	"testing"

	"github.com/colonyops/hive/internal/desktop/notify"
	"github.com/stretchr/testify/require"
)

type fakeNotificationNotifier struct {
	input     notify.Input
	notifyErr error
	status    string
	statusErr error
	granted   bool
	grantErr  error
}

func (n *fakeNotificationNotifier) Notify(in notify.Input) error {
	n.input = in
	return n.notifyErr
}

func (n *fakeNotificationNotifier) PermissionStatus() (string, error) {
	return n.status, n.statusErr
}

func (n *fakeNotificationNotifier) RequestPermission() (bool, error) {
	return n.granted, n.grantErr
}

func TestNotificationServiceForwardsNotifierCalls(t *testing.T) {
	notifier := &fakeNotificationNotifier{status: "granted", granted: true}
	service := NewNotificationService(notifier)

	err := service.Notify(NotifyInput{Title: "title", Subtitle: "subtitle", Body: "body", Severity: "warning", Sound: true, Data: map[string]any{"id": "1"}})
	require.NoError(t, err)
	require.Equal(t, notify.Input{Title: "title", Subtitle: "subtitle", Body: "body", Severity: "warning", Sound: true, Data: map[string]any{"id": "1"}}, notifier.input)

	status, err := service.PermissionStatus()
	require.NoError(t, err)
	require.Equal(t, "granted", status)
	granted, err := service.RequestNotificationPermission()
	require.NoError(t, err)
	require.True(t, granted)
}

func TestUnavailableNotificationServiceReturnsSetupError(t *testing.T) {
	service := NewUnavailableNotificationService(errors.New("icon cache is read-only"))

	err := service.Notify(NotifyInput{Title: "title"})
	require.ErrorContains(t, err, "native notifications unavailable")
	require.ErrorContains(t, err, "icon cache is read-only")
	_, err = service.PermissionStatus()
	require.ErrorContains(t, err, "native notifications unavailable")
	_, err = service.RequestNotificationPermission()
	require.ErrorContains(t, err, "native notifications unavailable")
}
