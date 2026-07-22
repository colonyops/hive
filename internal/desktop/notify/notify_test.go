package notify

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	wailsnotify "github.com/wailsapp/wails/v3/pkg/services/notifications"
)

type fakeSender struct {
	checkGranted   bool
	checkErr       error
	requestGranted bool
	requestErr     error
	sendErr        error

	checks   int
	requests int
	sent     []wailsnotify.NotificationOptions
}

func (s *fakeSender) SendNotification(options wailsnotify.NotificationOptions) error {
	s.sent = append(s.sent, options)
	return s.sendErr
}

func (s *fakeSender) CheckNotificationAuthorization() (bool, error) {
	s.checks++
	return s.checkGranted, s.checkErr
}

func (s *fakeSender) RequestNotificationAuthorization() (bool, error) {
	s.requests++
	return s.requestGranted, s.requestErr
}

func TestMaterializeIcon(t *testing.T) {
	cacheDir := t.TempDir()
	path, err := materializeIcon(cacheDir, []byte("first"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(cacheDir, "hive", iconFilename), path)
	require.True(t, filepath.IsAbs(path))
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, []byte("first"), contents)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o644), info.Mode().Perm())

	second, err := materializeIcon(cacheDir, []byte("replacement"))
	require.NoError(t, err)
	require.Equal(t, path, second)
	contents, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, []byte("first"), contents, "an existing icon must not be rewritten")
}

func TestMaterializeIconRejectsNonRegularPath(t *testing.T) {
	cacheDir := t.TempDir()
	path := filepath.Join(cacheDir, "hive", iconFilename)
	require.NoError(t, os.MkdirAll(path, 0o755))

	_, err := materializeIcon(cacheDir, []byte("icon"))
	require.ErrorContains(t, err, "not a regular file")
}

func TestNotifyMapsInputToOptions(t *testing.T) {
	sender := &fakeSender{checkGranted: true}
	notifier, err := newNotifier(sender, []byte("icon"), t.TempDir())
	require.NoError(t, err)

	err = notifier.Notify(Input{
		Title:    "  Hive update  ",
		Subtitle: "subtitle",
		Body:     "body",
		Severity: "warning",
		Data:     map[string]any{"item": "abc"},
		Sound:    false,
	})
	require.NoError(t, err)
	require.Len(t, sender.sent, 1)

	got := sender.sent[0]
	require.NotEmpty(t, got.ID)
	require.Equal(t, "Hive update", got.Title)
	require.Equal(t, "Warning · subtitle", got.Subtitle)
	require.Equal(t, "timeSensitive", got.InterruptionLevel)
	require.Equal(t, "body", got.Body)
	require.Equal(t, map[string]any{"item": "abc"}, got.Data)
	require.Equal(t, &wailsnotify.NotificationSound{Silent: true}, got.Sound)
	require.Equal(t, []wailsnotify.NotificationAttachment{{Path: notifier.iconPath, Type: "appLogoOverride"}}, got.Attachments)
	require.True(t, filepath.IsAbs(got.Attachments[0].Path))
	require.Equal(t, 0, sender.requests)

	// macOS may move the attachment source into Notification Center. A later
	// delivery must recreate it rather than fail Wails' accessibility check.
	require.NoError(t, os.Remove(notifier.iconPath))
	err = notifier.Notify(Input{Title: "next", Severity: "success", Sound: true})
	require.NoError(t, err)
	recreated, err := os.ReadFile(notifier.iconPath)
	require.NoError(t, err)
	require.Equal(t, []byte("icon"), recreated)
	require.Len(t, sender.sent, 2)
	require.NotEqual(t, got.ID, sender.sent[1].ID)
	require.Nil(t, sender.sent[1].Sound)
	require.Equal(t, "Success", sender.sent[1].Subtitle)
	require.Equal(t, "active", sender.sent[1].InterruptionLevel)
}

func TestNativeSeverityUsesVisibleSystemDelivery(t *testing.T) {
	tests := []struct {
		severity, subtitle, wantSubtitle, wantLevel string
	}{
		{"info", "", "", "active"},
		{"success", "", "Success", "active"},
		{"warning", "Hive", "Warning · Hive", "timeSensitive"},
		{"error", "", "Error", "timeSensitive"},
	}
	for _, tt := range tests {
		subtitle, level, err := nativeSeverity(tt.severity, tt.subtitle)
		require.NoError(t, err)
		require.Equal(t, tt.wantSubtitle, subtitle)
		require.Equal(t, tt.wantLevel, level)
	}
}

func TestNotifyRejectsInvalidSeverityBeforeCheckingPermission(t *testing.T) {
	sender := &fakeSender{checkGranted: true}
	notifier, err := newNotifier(sender, []byte("icon"), t.TempDir())
	require.NoError(t, err)

	err = notifier.Notify(Input{Title: "bad", Severity: "urgent"})
	require.ErrorContains(t, err, "invalid notification severity")
	require.Zero(t, sender.checks)
	require.Empty(t, sender.sent)
}

func TestNotifyRejectsEmptyTitle(t *testing.T) {
	sender := &fakeSender{checkGranted: true}
	notifier, err := newNotifier(sender, []byte("icon"), t.TempDir())
	require.NoError(t, err)

	err = notifier.Notify(Input{Title: " \t "})
	require.ErrorContains(t, err, "title is required")
	require.Zero(t, sender.checks)
	require.Empty(t, sender.sent)
}

func TestPermissionStatusTriStateAndLazyRequest(t *testing.T) {
	sender := &fakeSender{requestGranted: false}
	notifier, err := newNotifier(sender, []byte("icon"), t.TempDir())
	require.NoError(t, err)

	status, err := notifier.PermissionStatus()
	require.NoError(t, err)
	require.Equal(t, permissionNotRequested, status)

	err = notifier.Notify(Input{Title: "requires permission"})
	require.ErrorContains(t, err, "permission denied")
	require.Equal(t, 1, sender.requests)

	status, err = notifier.PermissionStatus()
	require.NoError(t, err)
	require.Equal(t, permissionDenied, status)

	err = notifier.Notify(Input{Title: "still denied"})
	require.ErrorContains(t, err, "permission denied")
	require.Equal(t, 1, sender.requests, "denied permission must not be requested repeatedly")
}

func TestPermissionStatusGrantedAndRequestError(t *testing.T) {
	sender := &fakeSender{checkGranted: true}
	notifier, err := newNotifier(sender, []byte("icon"), t.TempDir())
	require.NoError(t, err)

	status, err := notifier.PermissionStatus()
	require.NoError(t, err)
	require.Equal(t, permissionGranted, status)

	sender.requestErr = errors.New("native unavailable")
	granted, err := notifier.RequestPermission()
	require.False(t, granted)
	require.ErrorContains(t, err, "request notification permission")
}
