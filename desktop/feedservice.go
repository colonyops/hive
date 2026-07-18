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

// Refresh refetches the sources the profile's feeds reference, bypassing the
// search cache TTL, and reports whether anything changed. Backs the manual
// "Refresh now" action.
func (s *FeedService) Refresh(profileID string) (bool, error) {
	return s.provider.Refresh(context.Background(), profileID)
}

// Sources returns the top-level source definitions, for the feed editor's
// source picker.
func (s *FeedService) Sources() ([]feed.SourceDef, error) {
	return s.provider.Sources(context.Background())
}

// FeedDefFor returns one feed's definition, for edit prefill.
func (s *FeedService) FeedDefFor(profileID, feedID string) (feed.FeedDef, error) {
	return s.provider.FeedDefFor(context.Background(), profileID, feedID)
}

// CreateSource persists a new top-level source and returns it with its
// assigned ID.
func (s *FeedService) CreateSource(def feed.SourceDef) (feed.SourceDef, error) {
	return s.provider.CreateSource(context.Background(), def)
}

// CreateFeed persists a new feed in the profile (ID derived from the name)
// and returns the feed's materialized summary.
func (s *FeedService) CreateFeed(profileID string, def feed.FeedDef) (feed.Source, error) {
	return s.provider.CreateFeed(context.Background(), profileID, def)
}

// UpdateFeed replaces the feed's definition; the feed keeps its ID.
func (s *FeedService) UpdateFeed(profileID, feedID string, def feed.FeedDef) error {
	return s.provider.UpdateFeed(context.Background(), profileID, feedID, def)
}

// Config describes the profiles config file: path, content, validity.
func (s *FeedService) Config() (feed.ConfigInfo, error) {
	return s.provider.Config(context.Background())
}

// ConfigPrompt returns a paste-ready prompt for a coding agent to edit the
// profiles config on the user's behalf.
func (s *FeedService) ConfigPrompt() (string, error) {
	info, err := s.provider.Config(context.Background())
	if err != nil {
		return "", err
	}
	return feed.BuildConfigPrompt(info), nil
}
