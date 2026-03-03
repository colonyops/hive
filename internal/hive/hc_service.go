package hive

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/rs/zerolog"
)

// HoneycombService orchestrates hc item and comment operations.
type HoneycombService struct {
	store  hc.Store
	logger zerolog.Logger
}

// NewHoneycombService creates a new HoneycombService.
func NewHoneycombService(store hc.Store, logger zerolog.Logger) *HoneycombService {
	return &HoneycombService{
		store:  store,
		logger: logger,
	}
}

// CreateItem creates a single hc item, resolving parent relationships when a
// ParentID is supplied.
func (s *HoneycombService) CreateItem(ctx context.Context, repoKey string, input hc.CreateItemInput) (hc.Item, error) {
	if input.Title == "" {
		return hc.Item{}, fmt.Errorf("title is required")
	}

	now := time.Now()
	item := hc.Item{
		ID:        hc.GenerateID(),
		RepoKey:   repoKey,
		Title:     input.Title,
		Desc:      input.Desc,
		Type:      input.Type,
		ParentID:  input.ParentID,
		Status:    hc.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if input.ParentID != "" {
		parent, err := s.store.GetItem(ctx, input.ParentID)
		if err != nil {
			return hc.Item{}, fmt.Errorf("get parent item %q: %w", input.ParentID, err)
		}

		if parent.IsEpic() {
			item.EpicID = parent.ID
			item.Depth = 1
		} else {
			item.EpicID = parent.EpicID
			item.Depth = parent.Depth + 1
		}
	}

	if err := s.store.CreateItems(ctx, []hc.Item{item}); err != nil {
		return hc.Item{}, fmt.Errorf("create item: %w", err)
	}

	return item, nil
}

// walkCreateInput yields hc.Items from a CreateInput tree in BFS order
// (parent before children), generating IDs and resolving relationships.
func walkCreateInput(input hc.CreateInput, repoKey, epicID, parentID string, depth int, now time.Time) iter.Seq[hc.Item] {
	return func(yield func(hc.Item) bool) {
		id := hc.GenerateID()
		item := hc.Item{
			ID:        id,
			RepoKey:   repoKey,
			EpicID:    epicID,
			ParentID:  parentID,
			Title:     input.Title,
			Desc:      input.Desc,
			Type:      input.Type,
			Status:    hc.StatusOpen,
			Depth:     depth,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if !yield(item) {
			return
		}
		childEpicID := epicID
		if depth == 0 {
			childEpicID = id
		}
		for _, child := range input.Children {
			for childItem := range walkCreateInput(child, repoKey, childEpicID, id, depth+1, now) {
				if !yield(childItem) {
					return
				}
			}
		}
	}
}

// CreateBulk walks a CreateInput tree (BFS) and persists all items in one
// atomic call. The root node must be of type epic.
func (s *HoneycombService) CreateBulk(ctx context.Context, repoKey string, input hc.CreateInput) ([]hc.Item, error) {
	if input.Type != hc.ItemTypeEpic {
		return nil, fmt.Errorf("root item must be of type epic, got %q", input.Type)
	}

	now := time.Now()
	var items []hc.Item
	for item := range walkCreateInput(input, repoKey, "", "", 0, now) {
		items = append(items, item)
	}

	if err := s.store.CreateItems(ctx, items); err != nil {
		return nil, fmt.Errorf("bulk create items: %w", err)
	}

	return items, nil
}

// GetItem returns an item by ID.
func (s *HoneycombService) GetItem(ctx context.Context, id string) (hc.Item, error) {
	return s.store.GetItem(ctx, id)
}

// UpdateItem applies a partial update to an item and returns the result.
func (s *HoneycombService) UpdateItem(ctx context.Context, id string, update hc.ItemUpdate) (hc.Item, error) {
	return s.store.UpdateItem(ctx, id, update)
}

// ListItems returns items matching the supplied filter.
func (s *HoneycombService) ListItems(ctx context.Context, filter hc.ListFilter) ([]hc.Item, error) {
	return s.store.ListItems(ctx, filter)
}

// Next returns the next actionable item for the given filter.
func (s *HoneycombService) Next(ctx context.Context, filter hc.NextFilter) (hc.Item, bool, error) {
	return s.store.NextItem(ctx, filter)
}

// ListComments returns all comments for an item in chronological order.
func (s *HoneycombService) ListComments(ctx context.Context, itemID string) ([]hc.Comment, error) {
	return s.store.ListComments(ctx, itemID)
}

// AddComment attaches a new comment to an item and returns the created comment.
func (s *HoneycombService) AddComment(ctx context.Context, itemID, message string) (hc.Comment, error) {
	if strings.TrimSpace(message) == "" {
		return hc.Comment{}, fmt.Errorf("message is required")
	}

	if _, err := s.store.GetItem(ctx, itemID); err != nil {
		return hc.Comment{}, fmt.Errorf("hc item %q: %w", itemID, hc.ErrNotFound)
	}

	comment := hc.Comment{
		ID:        hc.GenerateCommentID(),
		ItemID:    itemID,
		Message:   message,
		CreatedAt: time.Now(),
	}

	if err := s.store.AddComment(ctx, comment); err != nil {
		return hc.Comment{}, fmt.Errorf("add comment to item %q: %w", itemID, err)
	}

	return comment, nil
}

// Context assembles a ContextBlock for the given epic and session.
func (s *HoneycombService) Context(ctx context.Context, epicID, sessionID string) (hc.ContextBlock, error) {
	epic, err := s.store.GetItem(ctx, epicID)
	if err != nil {
		return hc.ContextBlock{}, fmt.Errorf("get epic %q: %w", epicID, err)
	}

	if !epic.IsEpic() {
		return hc.ContextBlock{}, fmt.Errorf("item %q is not an epic", epicID)
	}

	all, err := s.store.ListItems(ctx, hc.ListFilter{EpicID: epicID})
	if err != nil {
		return hc.ContextBlock{}, fmt.Errorf("list items for epic %q: %w", epicID, err)
	}

	var counts hc.TaskCounts
	var allOpen []hc.Item
	var myTasks []hc.TaskWithComment

	for _, item := range all {
		switch item.Status {
		case hc.StatusOpen:
			counts.Open++
		case hc.StatusInProgress:
			counts.InProgress++
		case hc.StatusDone:
			counts.Done++
		case hc.StatusCancelled:
			counts.Cancelled++
		}

		if (item.Status == hc.StatusOpen || item.Status == hc.StatusInProgress) &&
			(sessionID == "" || item.SessionID != sessionID) {
			allOpen = append(allOpen, item)
		}

		if sessionID != "" && item.SessionID == sessionID && (item.Status == hc.StatusOpen || item.Status == hc.StatusInProgress) {
			twc := hc.TaskWithComment{Item: item}

			comments, err := s.store.ListComments(ctx, item.ID)
			if err != nil {
				s.logger.Warn().Err(err).Str("item_id", item.ID).Msg("failed to list comments for my task")
			} else if len(comments) > 0 {
				twc.LatestComment = comments[len(comments)-1]
			}

			myTasks = append(myTasks, twc)
		}
	}

	return hc.ContextBlock{
		Epic:         epic,
		Counts:       counts,
		MyTasks:      myTasks,
		AllOpenTasks: allOpen,
	}, nil
}

// ListRepoKeys returns all distinct, non-empty repo keys.
func (s *HoneycombService) ListRepoKeys(ctx context.Context) ([]string, error) {
	return s.store.ListRepoKeys(ctx)
}

// Prune delegates to the store's Prune implementation.
func (s *HoneycombService) Prune(ctx context.Context, opts hc.PruneOpts) (int, error) {
	return s.store.Prune(ctx, opts)
}
