package main

import (
	"context"

	"github.com/colonyops/hive/internal/desktop/feed"
)

// FeedService is the Wails service exposing feed data to the frontend. All
// data comes from the injected feed.Provider; this type is wire glue only.
type FeedService struct {
	provider feed.Provider
}

func NewFeedService(provider feed.Provider) *FeedService {
	return &FeedService{provider: provider}
}

// Profiles returns the profiles shown in the desktop rail.
func (s *FeedService) Profiles() ([]feed.Profile, error) {
	return s.provider.Profiles(context.Background())
}

// Items returns the items of one feed, or of every feed in the profile when
// feedID is empty.
func (s *FeedService) Items(profileID, feedID string) ([]feed.Item, error) {
	return s.provider.Items(context.Background(), profileID, feedID)
}

// ActionsFor returns the actions available for a PR or issue.
func (s *FeedService) ActionsFor(kind string) []feed.Action {
	return s.provider.ActionsFor(kind)
}

// MarkRead records an item as read app-locally.
func (s *FeedService) MarkRead(profileID, itemID string) error {
	return s.provider.MarkRead(context.Background(), profileID, itemID)
}

// CreateProfile creates a workspace seeded with the default feeds.
func (s *FeedService) CreateProfile(name string) (feed.Profile, error) {
	return s.provider.CreateProfile(context.Background(), name)
}

// Refresh refetches the profile's feeds, bypassing the cache TTL, and
// reports whether anything changed. Backs the manual "Refresh now" action.
func (s *FeedService) Refresh(profileID string) (bool, error) {
	return s.provider.Refresh(context.Background(), profileID)
}
