package main

import (
	"errors"
	"fmt"

	"github.com/colonyops/hive/internal/desktop/notify"
)

// NotifyInput is the frontend-facing request for a native notification.
type NotifyInput struct {
	Title    string         `json:"title"`
	Subtitle string         `json:"subtitle"`
	Body     string         `json:"body"`
	Sound    bool           `json:"sound"`
	Data     map[string]any `json:"data"`
}

// notificationNotifier is the small notification API exposed to the binding.
type notificationNotifier interface {
	Notify(notify.Input) error
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

// Notify sends a native notification.
func (s *NotificationService) Notify(in NotifyInput) error {
	return s.notifier.Notify(notify.Input{
		Title:    in.Title,
		Subtitle: in.Subtitle,
		Body:     in.Body,
		Sound:    in.Sound,
		Data:     in.Data,
	})
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

type unavailableNotifier struct {
	err error
}

func (n unavailableNotifier) Notify(notify.Input) error {
	return fmt.Errorf("native notifications unavailable: %w", n.err)
}

func (n unavailableNotifier) PermissionStatus() (string, error) {
	return "", fmt.Errorf("native notifications unavailable: %w", n.err)
}

func (n unavailableNotifier) RequestPermission() (bool, error) {
	return false, fmt.Errorf("native notifications unavailable: %w", n.err)
}
