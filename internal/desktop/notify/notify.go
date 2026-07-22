// Package notify adapts Hive notification requests to Wails native notifications.
package notify

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/colonyops/hive/pkg/randid"
	wailsnotify "github.com/wailsapp/wails/v3/pkg/services/notifications"
)

const (
	permissionGranted      = "granted"
	permissionDenied       = "denied"
	permissionNotRequested = "not-requested"
	iconFilename           = "notification-icon.png"
)

// sender is the subset of Wails' notification service used by Notifier.
type sender interface {
	SendNotification(wailsnotify.NotificationOptions) error
	CheckNotificationAuthorization() (bool, error)
	RequestNotificationAuthorization() (bool, error)
}

// Input is a transport-neutral request for a native notification.
type Input struct {
	Title    string
	Subtitle string
	Body     string
	Sound    bool
	Data     map[string]any
}

// Notifier builds Wails notification options, manages native authorization, and
// sends notifications through its Wails sender. It is safe for concurrent use.
type Notifier struct {
	sender   sender
	iconPath string

	mu        sync.Mutex
	requested bool
}

// New constructs a Notifier and materializes iconPNG at the stable per-user
// cache path needed by Wails notification attachments.
func New(svc sender, iconPNG []byte) (*Notifier, error) {
	if svc == nil {
		return nil, errors.New("notification sender is required")
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user cache directory: %w", err)
	}
	return newNotifier(svc, iconPNG, cacheDir)
}

func newNotifier(svc sender, iconPNG []byte, cacheDir string) (*Notifier, error) {
	if svc == nil {
		return nil, errors.New("notification sender is required")
	}
	iconPath, err := materializeIcon(cacheDir, iconPNG)
	if err != nil {
		return nil, err
	}
	return &Notifier{sender: svc, iconPath: iconPath}, nil
}

// Notify validates in, requests authorization only when it has not been
// explicitly requested before, and sends the resulting native notification.
func (n *Notifier) Notify(in Input) error {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return errors.New("notification title is required")
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	status, err := n.permissionStatusLocked()
	if err != nil {
		return err
	}
	switch status {
	case permissionNotRequested:
		granted, err := n.requestPermissionLocked()
		if err != nil {
			return err
		}
		if !granted {
			return errors.New("notification permission denied")
		}
	case permissionDenied:
		return errors.New("notification permission denied")
	}

	options := wailsnotify.NotificationOptions{
		ID:       "hive-" + randid.Generate(24),
		Title:    title,
		Subtitle: in.Subtitle,
		Body:     in.Body,
		Data:     in.Data,
		Attachments: []wailsnotify.NotificationAttachment{{
			Path: n.iconPath,
			Type: "appLogoOverride",
		}},
	}
	if !in.Sound {
		options.Sound = &wailsnotify.NotificationSound{Silent: true}
	}
	if err := n.sender.SendNotification(options); err != nil {
		return fmt.Errorf("send notification: %w", err)
	}
	return nil
}

// PermissionStatus reports granted, denied, or not-requested. Wails exposes
// only a boolean authorization state, so an unauthorized service is considered
// not-requested until this Notifier has made an explicit permission request.
func (n *Notifier) PermissionStatus() (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.permissionStatusLocked()
}

// RequestPermission explicitly requests native notification authorization and
// records that a request was made so future unauthorized checks mean denied.
func (n *Notifier) RequestPermission() (bool, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.requestPermissionLocked()
}

func (n *Notifier) permissionStatusLocked() (string, error) {
	granted, err := n.sender.CheckNotificationAuthorization()
	if err != nil {
		return "", fmt.Errorf("check notification permission: %w", err)
	}
	if granted {
		return permissionGranted, nil
	}
	if n.requested {
		return permissionDenied, nil
	}
	return permissionNotRequested, nil
}

func (n *Notifier) requestPermissionLocked() (bool, error) {
	granted, err := n.sender.RequestNotificationAuthorization()
	if err != nil {
		return false, fmt.Errorf("request notification permission: %w", err)
	}
	n.requested = true
	return granted, nil
}

func materializeIcon(cacheDir string, iconPNG []byte) (string, error) {
	path, err := filepath.Abs(filepath.Join(cacheDir, "hive", iconFilename))
	if err != nil {
		return "", fmt.Errorf("resolve notification icon path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create notification icon directory: %w", err)
	}

	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("notification icon path is not a regular file: %s", path)
		}
		return path, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat notification icon: %w", err)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return materializeIcon(cacheDir, iconPNG)
	}
	if err != nil {
		return "", fmt.Errorf("create notification icon: %w", err)
	}
	if _, err := file.Write(iconPNG); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write notification icon: %w", err)
	}
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("set notification icon permissions: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close notification icon: %w", err)
	}
	return path, nil
}
