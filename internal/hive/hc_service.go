package hive

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/pkg/randid"
	"github.com/rs/zerolog"
)

// HCService orchestrates honeycomb task operations.
type HCService struct {
	store    hc.Store
	messages *MessageService
	logger   zerolog.Logger
}

// NewHCService creates a new HCService.
func NewHCService(store hc.Store, messages *MessageService, logger zerolog.Logger) *HCService {
	return &HCService{
		store:    store,
		messages: messages,
		logger:   logger.With().Str("component", "hc").Logger(),
	}
}

// generateHCID generates a short unique ID for an HC item.
func generateHCID() string {
	return "hc-" + randid.Generate(8)
}

// CreateItem creates a single hc item.
func (s *HCService) CreateItem(ctx context.Context, item hc.Item) error {
	return s.store.CreateItem(ctx, item)
}

// CreateBulk walks the CreateInput tree BFS and creates all items atomically.
// The caller provides the repo key and session ID; IDs are generated here.
// The root node is expected to be an epic; all children reference it as EpicID.
func (s *HCService) CreateBulk(ctx context.Context, input hc.CreateInput, repoKey, sessionID string) ([]hc.Item, error) {
	now := time.Now()
	var items []hc.Item

	// Generate the root ID first so children can reference it as EpicID.
	rootID := generateHCID()
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
			id := generateHCID()
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
func (s *HCService) GetItem(ctx context.Context, id string) (hc.Item, error) {
	return s.store.GetItem(ctx, id)
}

// UpdateItem updates an item's status and/or session assignment.
// On success, publishes a non-fatal activity notification to the epic's topic.
func (s *HCService) UpdateItem(ctx context.Context, id string, update hc.ItemUpdate) (hc.Item, error) {
	item, err := s.store.UpdateItem(ctx, id, update)
	if err != nil {
		return hc.Item{}, fmt.Errorf("update hc item %q: %w", id, err)
	}

	if item.EpicID != "" {
		s.publishActivity(ctx, item.EpicID, item)
	}

	return item, nil
}

// ListItems returns items matching the filter.
func (s *HCService) ListItems(ctx context.Context, filter hc.ListFilter) ([]hc.Item, error) {
	return s.store.ListItems(ctx, filter)
}

// Next returns the next ready leaf task for the session (no open children).
func (s *HCService) Next(ctx context.Context, filter hc.NextFilter) (hc.Item, bool, error) {
	return s.store.NextItem(ctx, filter)
}

// LogActivity records an activity entry for an item.
func (s *HCService) LogActivity(ctx context.Context, itemID string, actType hc.ActivityType, message string) (hc.Activity, error) {
	a := hc.Activity{
		ID:        generateHCID(),
		ItemID:    itemID,
		Type:      actType,
		Message:   message,
		CreatedAt: time.Now(),
	}
	if err := s.store.LogActivity(ctx, a); err != nil {
		return hc.Activity{}, fmt.Errorf("log hc activity for %q: %w", itemID, err)
	}

	// Publish to epic's activity topic (non-fatal).
	item, err := s.store.GetItem(ctx, itemID)
	if err == nil && item.EpicID != "" {
		s.publishActivity(ctx, item.EpicID, item)
	}

	return a, nil
}

// Checkpoint records a checkpoint activity for an item.
func (s *HCService) Checkpoint(ctx context.Context, itemID, message string) (hc.Activity, error) {
	return s.LogActivity(ctx, itemID, hc.ActivityTypeCheckpoint, message)
}

// Context assembles an HCContextBlock for the given epic.
func (s *HCService) Context(ctx context.Context, epicID, sessionID string) (HCContextBlock, error) {
	epic, err := s.store.GetItem(ctx, epicID)
	if err != nil {
		return HCContextBlock{}, fmt.Errorf("get epic %q: %w", epicID, err)
	}

	all, err := s.store.ListItems(ctx, hc.ListFilter{EpicID: epicID})
	if err != nil {
		return HCContextBlock{}, fmt.Errorf("list hc items for epic %q: %w", epicID, err)
	}

	counts := HCTaskCounts{}
	var myTasks []HCTaskWithCheckpoint
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
			myTasks = append(myTasks, HCTaskWithCheckpoint{
				Item:             item,
				LatestCheckpoint: checkpoint,
			})
		}
	}

	return HCContextBlock{
		Epic:         epic,
		Counts:       counts,
		MyTasks:      myTasks,
		AllOpenTasks: allOpen,
	}, nil
}

// ListActivity returns all activity entries for an item.
func (s *HCService) ListActivity(ctx context.Context, itemID string) ([]hc.Activity, error) {
	return s.store.ListActivity(ctx, itemID)
}

// Prune removes old done/cancelled items and their activity.
func (s *HCService) Prune(ctx context.Context, opts hc.PruneOpts) (int, error) {
	return s.store.Prune(ctx, opts)
}

// publishActivity publishes a notification to hc.<epic-id>.activity topic.
// Errors are logged at debug level and not returned (non-fatal).
func (s *HCService) publishActivity(ctx context.Context, epicID string, item hc.Item) {
	topic := fmt.Sprintf("hc.%s.activity", epicID)
	payload := fmt.Sprintf(`{"item_id":%q,"status":%q}`, item.ID, item.Status)
	_, err := s.messages.Publish(ctx, messaging.Message{
		ID:      generateHCID(),
		Payload: payload,
	}, []string{topic})
	if err != nil {
		s.logger.Debug().Err(err).Str("topic", topic).Msg("failed to publish hc activity")
	}
}

// HCContextBlock is the assembled context view for an epic.
type HCContextBlock struct {
	Epic         hc.Item
	Counts       HCTaskCounts
	MyTasks      []HCTaskWithCheckpoint
	AllOpenTasks []hc.Item
}

// HCTaskCounts holds counts of items by status.
type HCTaskCounts struct {
	Open       int
	InProgress int
	Done       int
	Cancelled  int
}

// HCTaskWithCheckpoint pairs an item with its latest checkpoint activity.
type HCTaskWithCheckpoint struct {
	Item             hc.Item
	LatestCheckpoint hc.Activity
}
