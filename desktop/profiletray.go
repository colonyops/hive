package main

import (
	"sync"

	"github.com/colonyops/hive/internal/desktop/pipeline/flow"
	"github.com/rs/zerolog"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type trayProfile struct {
	ID      string
	Label   string
	Enabled bool
	Valid   bool
}

func trayProfiles(store *flow.FlowStore) []trayProfile {
	statuses := store.Statuses()
	profiles := make([]trayProfile, 0, len(statuses))
	for _, status := range statuses {
		if status.Valid {
			profiles = append(profiles, trayProfile{
				ID:      status.ID,
				Label:   status.Flow.Name,
				Enabled: status.Flow.Enabled,
				Valid:   true,
			})
			continue
		}
		profiles = append(profiles, trayProfile{
			ID:    status.ID,
			Label: status.ID + " (invalid)",
		})
	}
	return profiles
}

// profileTray owns the dynamic native tray menu. Profile rows are checkboxes:
// checked profiles poll and run, while unchecked profiles retain their feed
// data without executing. Invalid flow files remain visible but non-interactive.
type profileTray struct {
	app       *application.App
	store     *flow.FlowStore
	logger    zerolog.Logger
	onUpdated func()
	show      func()
	quit      func()
	tray      *application.SystemTray
	mu        sync.Mutex
	active    bool
}

func newProfileTray(
	app *application.App,
	store *flow.FlowStore,
	logger zerolog.Logger,
	icon []byte,
	onUpdated func(),
	show func(),
	quit func(),
) *profileTray {
	result := &profileTray{
		app:       app,
		store:     store,
		logger:    logger,
		onUpdated: onUpdated,
		show:      show,
		quit:      quit,
		active:    true,
	}
	result.tray = app.SystemTray.New().SetTemplateIcon(icon)
	result.Refresh()
	return result
}

// Refresh replaces the tray menu from the current flow-store snapshot. Wails
// marshals SetMenu onto the native UI thread after app startup.
func (t *profileTray) Refresh() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.active {
		return
	}
	t.tray.SetMenu(t.menu())
}

// Close prevents filesystem watcher callbacks from touching the native tray
// once Wails begins tearing down its UI loop.
func (t *profileTray) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active = false
}

func (t *profileTray) menu() *application.Menu {
	menu := t.app.NewMenu()
	menu.Add("Show Hive").OnClick(func(*application.Context) { t.show() })
	menu.AddSeparator()

	for _, profile := range trayProfiles(t.store) {
		item := menu.AddCheckbox(profile.Label, profile.Enabled).SetEnabled(profile.Valid)
		if !profile.Valid {
			continue
		}
		id := profile.ID
		enabled := !profile.Enabled
		item.OnClick(func(*application.Context) {
			if _, err := t.store.SetEnabled(id, enabled); err != nil {
				t.logger.Warn().Err(err).Str("profile", id).Msg("tray: updating profile enablement failed")
				return
			}
			if t.onUpdated != nil {
				t.onUpdated()
			}
		})
	}

	menu.AddSeparator()
	menu.Add("Quit").OnClick(func(*application.Context) { t.quit() })
	return menu
}
