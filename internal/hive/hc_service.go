package hive

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/rs/zerolog"
)

// HoneycombService orchestrates honeycomb task operations.
type HoneycombService struct {
	store  hc.Store
	logger zerolog.Logger
}

// NewHoneycombService creates a new HoneycombService.
func NewHoneycombService(store hc.Store, logger zerolog.Logger) *HoneycombService {
	return &HoneycombService{
		store:  store,
		logger: logger.With().Str("component", "honeycomb").Logger(),
	}
}

// CreateItem creates a single hc item from a domain input DTO.
func (s *HoneycombService) CreateItem(ctx context.Context, input hc.CreateItemInput, repoKey, sessionID string) (hc.Item, error) {
	if input.Title == "" {
		return hc.Item{}, fmt.Errorf("title is required")
	}

	now := time.Now()
	item := hc.Item{
		ID:        hc.GenerateID(),
		RepoKey:   repoKey,
		ParentID:  input.ParentID,
		SessionID: sessionID,
		Title:     input.Title,
		Desc:      input.Desc,
		Type:      input.Type,
		Status:    hc.StatusOpen,
		Depth:     0,
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
		} else {
			item.EpicID = parent.EpicID
		}
		item.Depth = parent.Depth + 1
	}

	if err := s.store.CreateItem(ctx, item); err != nil {
		return hc.Item{}, fmt.Errorf("create hc item: %w", err)
	}

	return item, nil
}

// CreateBulk walks the CreateInput tree BFS and creates all items atomically.
// The caller provides the repo key and session ID; IDs are generated here.
// The root node is expected to be an epic; all children reference it as EpicID.
func (s *HoneycombService) CreateBulk(ctx context.Context, input hc.CreateInput, repoKey, sessionID string) ([]hc.Item, error) {
	now := time.Now()
	var items []hc.Item

	// Generate the root ID first so children can reference it as EpicID.
	rootID := hc.GenerateID()
	rootItem := hc.Item{
		ID:        rootID,
		RepoKey:   repoKey,
		EpicID:    "",
		ParentID:  "",
		SessionID: sessionID,
		Title:     input.Title,
		Desc:      input.Desc,
		Type:      input.Type,
		Status:    hc.StatusOpen,
		Depth:     0,
		CreatedAt: now,
		UpdatedAt: now,
	}
	items = append(items, rootItem)

	// BFS for children.
	type queueEntry struct {
		nodes    []hc.CreateInput
		parentID string
		depth    int
	}

	queue := []queueEntry{{nodes: input.Children, parentID: rootID, depth: 1}}
	for len(queue) > 0 {
		entry := queue[0]
		queue = queue[1:]
		for _, node := range entry.nodes {
			id := hc.GenerateID()
			item := hc.Item{
				ID:        id,
				RepoKey:   repoKey,
				EpicID:    rootID,
				ParentID:  entry.parentID,
				SessionID: sessionID,
				Title:     node.Title,
				Desc:      node.Desc,
				Type:      node.Type,
				Status:    hc.StatusOpen,
				Depth:     entry.depth,
				CreatedAt: now,
				UpdatedAt: now,
			}
			items = append(items, item)
			if len(node.Children) > 0 {
				queue = append(queue, queueEntry{nodes: node.Children, parentID: id, depth: entry.depth + 1})
			}
		}
	}

	if err := s.store.CreateItemBatch(ctx, items); err != nil {
		return nil, fmt.Errorf("create hc items batch: %w", err)
	}

	return items, nil
}

// GetItem retrieves a single item by ID.
func (s *HoneycombService) GetItem(ctx context.Context, id string) (hc.Item, error) {
	return s.store.GetItem(ctx, id)
}

// UpdateItem updates an item's status and/or session assignment.
func (s *HoneycombService) UpdateItem(ctx context.Context, id string, update hc.ItemUpdate) (hc.Item, error) {
	item, err := s.store.UpdateItem(ctx, id, update)
	if err != nil {
		return hc.Item{}, fmt.Errorf("update hc item %q: %w", id, err)
	}
	return item, nil
}

// ListItems returns items matching the filter.
func (s *HoneycombService) ListItems(ctx context.Context, filter hc.ListFilter) ([]hc.Item, error) {
	return s.store.ListItems(ctx, filter)
}

// Next returns the next ready leaf task for the session (no open children).
func (s *HoneycombService) Next(ctx context.Context, filter hc.NextFilter) (hc.Item, bool, error) {
	return s.store.NextItem(ctx, filter)
}

// LogActivity records an activity entry for an item.
func (s *HoneycombService) LogActivity(ctx context.Context, itemID string, actType hc.ActivityType, message string) (hc.Activity, error) {
	a := hc.Activity{
		ID:        hc.GenerateID(),
		ItemID:    itemID,
		Type:      actType,
		Message:   message,
		CreatedAt: time.Now(),
	}
	if err := s.store.LogActivity(ctx, a); err != nil {
		return hc.Activity{}, fmt.Errorf("log hc activity for %q: %w", itemID, err)
	}
	return a, nil
}

// Checkpoint records a checkpoint activity for an item.
func (s *HoneycombService) Checkpoint(ctx context.Context, itemID, message string) (hc.Activity, error) {
	return s.LogActivity(ctx, itemID, hc.ActivityTypeCheckpoint, message)
}

// Context assembles a context block for the given epic.
func (s *HoneycombService) Context(ctx context.Context, epicID, sessionID string) (hc.ContextBlock, error) {
	epic, err := s.store.GetItem(ctx, epicID)
	if err != nil {
		return hc.ContextBlock{}, fmt.Errorf("get epic %q: %w", epicID, err)
	}

	all, err := s.store.ListItems(ctx, hc.ListFilter{EpicID: epicID})
	if err != nil {
		return hc.ContextBlock{}, fmt.Errorf("list hc items for epic %q: %w", epicID, err)
	}

	counts := hc.TaskCounts{}
	var myTasks []hc.TaskWithCheckpoint
	var allOpen []hc.Item

	for _, item := range all {
		switch item.Status {
		case hc.StatusOpen:
			counts.Open++
			allOpen = append(allOpen, item)
		case hc.StatusInProgress:
			counts.InProgress++
			allOpen = append(allOpen, item)
		case hc.StatusDone:
			counts.Done++
		case hc.StatusCancelled:
			counts.Cancelled++
		}

		if item.SessionID == sessionID && (item.Status == hc.StatusOpen || item.Status == hc.StatusInProgress) {
			checkpoint, _, cpErr := s.store.LatestCheckpoint(ctx, item.ID)
			if cpErr != nil {
				s.logger.Debug().Err(cpErr).Str("item_id", item.ID).Msg("failed to fetch latest checkpoint")
			}
			myTasks = append(myTasks, hc.TaskWithCheckpoint{
				Item:             item,
				LatestCheckpoint: checkpoint,
			})
		}
	}

	return hc.ContextBlock{
		Epic:         epic,
		Counts:       counts,
		MyTasks:      myTasks,
		AllOpenTasks: allOpen,
	}, nil
}

// ListActivity returns all activity entries for an item.
func (s *HoneycombService) ListActivity(ctx context.Context, itemID string) ([]hc.Activity, error) {
	return s.store.ListActivity(ctx, itemID)
}

// Prune removes old done/cancelled items and their activity.
func (s *HoneycombService) Prune(ctx context.Context, opts hc.PruneOpts) (int, error) {
	return s.store.Prune(ctx, opts)
}
