package app

import (
	"cmp"
	"context"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

type MenuBarService struct {
	settings *settings.Store
	flows    *flow.FlowStore
	items    *stores.InboxItemStore
	actions  *actions.ActionStore
	polls    LastTicker
	events   *events.Bus
}

type LastTicker interface {
	LastTick() time.Time
}

type MenuBarDeps struct {
	Settings *settings.Store
	Flows    *flow.FlowStore
	Items    *stores.InboxItemStore
	Catalog  *actions.ActionStore
	Polls    LastTicker
	Events   *events.Bus
}

func newMenuBarService(d MenuBarDeps) *MenuBarService {
	return &MenuBarService{settings: d.Settings, flows: d.Flows, items: d.Items, actions: d.Catalog, polls: d.Polls, events: d.Events}
}

type MenuBarPin struct {
	Feed  string `json:"feed"`
	Limit int    `json:"limit"`
}

type MenuBarFeedChoice struct {
	Feed string `json:"feed"`
	MenuBarFeedName
}

// MenuBarFeedName uses an empty Folder for feeds at the sidebar's top level.
type MenuBarFeedName struct {
	ProfileName string `json:"profileName"`
	Folder      string `json:"folder"`
	Name        string `json:"name"`
}

type MenuBarSnapshot struct {
	Pinned     []MenuBarFeedView
	LastPolled time.Time
}

type MenuBarFeedView struct {
	ProfileID string
	Feed      string
	MenuBarFeedName
	Total int64
	Items []MenuBarItem
}

type MenuBarItem struct {
	ID      int64
	Title   string
	URL     string
	Unread  bool
	Actions []MenuBarAction
}

type MenuBarAction struct {
	ID        string
	Label     string
	Clipboard bool
}

func (s *MenuBarService) Pins(context.Context) ([]MenuBarPin, error) {
	cfg, err := s.settings.Effective()
	if err != nil {
		return nil, Wrap(err, KindInternal, "reading settings")
	}
	pins := make([]MenuBarPin, 0, len(cfg.MenuBar.Feeds))
	for _, feed := range cfg.MenuBar.Feeds {
		pins = append(pins, MenuBarPin{Feed: feed.Feed, Limit: feed.ItemLimit()})
	}
	return pins, nil
}

// SetPins preserves list order but does not require feeds to exist; missing
// feeds are skipped when read.
func (s *MenuBarService) SetPins(ctx context.Context, pins []MenuBarPin) error {
	next := settings.MenuBarSettings{Feeds: make([]settings.MenuBarFeed, 0, len(pins))}
	for _, pin := range pins {
		limit := pin.Limit
		if limit == settings.DefaultMenuBarItemLimit {
			limit = 0
		}
		next.Feeds = append(next.Feeds, settings.MenuBarFeed{Feed: pin.Feed, Limit: limit})
	}
	if err := next.Validate(); err != nil {
		return Wrap(err, KindInvalid, "pinning menu bar feeds")
	}
	if _, err := s.settings.Update(func(current *settings.Settings) error {
		current.MenuBar = next
		return nil
	}); err != nil {
		return Wrap(err, KindInternal, "saving the menu bar feeds")
	}
	s.events.Publish(ctx, events.MenuBarUpdated{})
	return nil
}

// FeedChoices preserves profile rail and feed declaration order.
func (s *MenuBarService) FeedChoices(context.Context) []MenuBarFeedChoice {
	choices := make([]MenuBarFeedChoice, 0)
	for _, f := range s.flows.List() {
		folders := s.feedFolders(f.ID)
		for _, node := range f.FeedNodes() {
			choices = append(choices, MenuBarFeedChoice{Feed: f.FeedID(node.ID), MenuBarFeedName: feedName(f, node, folders)})
		}
	}
	return choices
}

func (s *MenuBarService) Snapshot(ctx context.Context) (MenuBarSnapshot, error) {
	cfg, err := s.settings.Effective()
	if err != nil {
		return MenuBarSnapshot{}, Wrap(err, KindInternal, "reading settings")
	}
	counts := feedCountCache{items: s.items, byProfile: map[string]map[string]stores.FeedCount{}}
	runnable := s.runnableActions()

	snapshot := MenuBarSnapshot{Pinned: []MenuBarFeedView{}, LastPolled: s.LastPolled()}
	for _, pin := range cfg.MenuBar.Feeds {
		profileID, nodeID, _ := strings.Cut(pin.Feed, "/")
		f, ok := s.flows.Get(profileID)
		if !ok {
			continue
		}
		node, ok := feedNode(f, nodeID)
		if !ok {
			continue
		}
		count, err := counts.get(ctx, profileID, pin.Feed)
		if err != nil {
			return MenuBarSnapshot{}, err
		}
		rows, err := s.items.ListByFeed(ctx, profileID, pin.Feed, pin.ItemLimit())
		if err != nil {
			return MenuBarSnapshot{}, Wrap(err, KindInternal, "listing feed %q", pin.Feed)
		}
		view := MenuBarFeedView{
			ProfileID: profileID, Feed: pin.Feed, MenuBarFeedName: feedName(f, node, s.feedFolders(profileID)),
			Total: count.Total,
			Items: make([]MenuBarItem, 0, len(rows)),
		}
		for _, row := range rows {
			view.Items = append(view.Items, menuBarItem(row, runnable))
		}
		snapshot.Pinned = append(snapshot.Pinned, view)
	}

	return snapshot, nil
}

// LastPolled returns zero before the first poll and in mock modes.
func (s *MenuBarService) LastPolled() time.Time {
	if s.polls == nil {
		return time.Time{}
	}
	return s.polls.LastTick()
}

func (s *MenuBarService) runnableActions() []actions.Action {
	out := make([]actions.Action, 0)
	for _, action := range s.actions.List() {
		if action.ShowInDetail && action.RunsWithoutInput() {
			out = append(out, action)
		}
	}
	return out
}

func menuBarItem(row stores.InboxItem, runnable []actions.Action) MenuBarItem {
	item := MenuBarItem{
		ID: row.ID, Title: row.Title, URL: row.URL, Unread: row.Unread,
		Actions: []MenuBarAction{},
	}
	decoded, err := dispatch.DecodeActionItem(row.Payload, row.ExternalID)
	if err != nil {
		return item
	}
	for _, action := range runnable {
		if ok, _ := dispatch.ActionApplicability(action, decoded); !ok {
			continue
		}
		_, clipboard := action.Config.(*actions.ClipboardConfig)
		item.Actions = append(item.Actions, MenuBarAction{ID: action.ID, Label: action.Label, Clipboard: clipboard})
	}
	return item
}

// A missing sidebar layout leaves every feed at the top level.
func (s *MenuBarService) feedFolders(profileID string) map[string]string {
	folders := map[string]string{}
	for _, item := range s.flows.GetSidebar(profileID).Items {
		if item.Folder == nil {
			continue
		}
		for _, nodeID := range item.Folder.Feeds {
			folders[nodeID] = item.Folder.Name
		}
	}
	return folders
}

func feedName(f flow.Flow, node flow.Node, folders map[string]string) MenuBarFeedName {
	return MenuBarFeedName{ProfileName: cmp.Or(f.Name, f.ID), Folder: folders[node.ID], Name: cmp.Or(node.Name, node.ID)}
}

func feedNode(f flow.Flow, nodeID string) (flow.Node, bool) {
	for _, node := range f.FeedNodes() {
		if node.ID == nodeID {
			return node, true
		}
	}
	return flow.Node{}, false
}

type feedCountCache struct {
	items     *stores.InboxItemStore
	byProfile map[string]map[string]stores.FeedCount
}

func (c feedCountCache) get(ctx context.Context, profileID, feedID string) (stores.FeedCount, error) {
	byFeed, ok := c.byProfile[profileID]
	if !ok {
		counts, err := c.items.FeedCounts(ctx, profileID)
		if err != nil {
			return stores.FeedCount{}, Wrap(err, KindInternal, "counting feeds for %q", profileID)
		}
		byFeed = make(map[string]stores.FeedCount, len(counts))
		for _, count := range counts {
			byFeed[count.FeedID] = count
		}
		c.byProfile[profileID] = byFeed
	}
	return byFeed[feedID], nil
}
