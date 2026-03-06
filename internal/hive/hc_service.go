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

// createInputEntry pairs a CreateInput with its generated hc.Item.
type createInputEntry struct {
	input hc.CreateInput
	item  hc.Item
}

// walkCreateInputWithEntry yields (CreateInput, hc.Item) pairs from the tree in BFS order.
func walkCreateInputWithEntry(input hc.CreateInput, repoKey, epicID, parentID string, depth int, now time.Time) iter.Seq[createInputEntry] {
	return func(yield func(createInputEntry) bool) {
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
		if !yield(createInputEntry{input: input, item: item}) {
			return
		}
		childEpicID := epicID
		if depth == 0 {
			childEpicID = id
		}
		for _, child := range input.Children {
			for entry := range walkCreateInputWithEntry(child, repoKey, childEpicID, id, depth+1, now) {
				if !yield(entry) {
					return
				}
			}
		}
	}
}

// validateBatchBlockerRefs checks that all Blockers refs in the batch refer to known refs
// and that no in-batch cycles exist.
func validateBatchBlockerRefs(entries []createInputEntry) error {
	refToID := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.input.Ref != "" {
			refToID[e.input.Ref] = e.item.ID
		}
	}

	// Collect in-batch edges
	var edges [][2]string
	for _, e := range entries {
		for _, blocker := range e.input.Blockers {
			blockerID, ok := refToID[blocker]
			if !ok {
				return fmt.Errorf("unknown blocker ref %q", blocker)
			}
			edges = append(edges, [2]string{blockerID, e.item.ID})
		}
	}

	// Check for cycles among in-batch edges
	for i, edge := range edges {
		existing := make([][2]string, 0, len(edges)-1)
		for j, e := range edges {
			if j != i {
				existing = append(existing, e)
			}
		}
		if wouldCycle(existing, edge[0], edge[1]) {
			return fmt.Errorf("in-batch cycle detected involving ref edges")
		}
	}
	return nil
}

// CreateBulk walks a CreateInput tree (BFS) and persists all items in one
// atomic call. The root node must be of type epic.
func (s *HoneycombService) CreateBulk(ctx context.Context, repoKey string, input hc.CreateInput) ([]hc.Item, error) {
	if input.Type != hc.ItemTypeEpic {
		return nil, fmt.Errorf("root item must be of type epic, got %q", input.Type)
	}

	now := time.Now()
	var entries []createInputEntry
	for entry := range walkCreateInputWithEntry(input, repoKey, "", "", 0, now) {
		entries = append(entries, entry)
	}

	if err := validateBatchBlockerRefs(entries); err != nil {
		return nil, fmt.Errorf("validate blocker refs: %w", err)
	}

	items := make([]hc.Item, len(entries))
	for i, e := range entries {
		items[i] = e.item
	}

	if err := s.store.CreateItems(ctx, items); err != nil {
		return nil, fmt.Errorf("bulk create items: %w", err)
	}

	// Wire blocker edges
	refToID := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.input.Ref != "" {
			refToID[e.input.Ref] = e.item.ID
		}
	}
	for _, e := range entries {
		for _, blockerRef := range e.input.Blockers {
			blockerID := refToID[blockerRef]
			if err := s.store.AddBlocker(ctx, blockerID, e.item.ID); err != nil {
				s.logger.Warn().Err(err).Str("blocker", blockerID).Str("blocked", e.item.ID).Msg("failed to wire blocker edge")
			}
		}
	}

	return items, nil
}

// AddBlocker records that blockerID blocks blockedID.
// Returns hc.ErrCyclicDependency if adding this edge would create a cycle.
func (s *HoneycombService) AddBlocker(ctx context.Context, blockerID, blockedID string) error {
	if blockerID == blockedID {
		return hc.ErrCyclicDependency
	}

	edges, err := s.store.ListBlockerEdges(ctx)
	if err != nil {
		return fmt.Errorf("list blocker edges: %w", err)
	}

	if wouldCycle(edges, blockerID, blockedID) {
		return hc.ErrCyclicDependency
	}

	return s.store.AddBlocker(ctx, blockerID, blockedID)
}

// RemoveBlocker removes the explicit blocker relationship.
func (s *HoneycombService) RemoveBlocker(ctx context.Context, blockerID, blockedID string) error {
	return s.store.RemoveBlocker(ctx, blockerID, blockedID)
}

// ListBlockers returns IDs of open/in_progress items that explicitly block the given item.
func (s *HoneycombService) ListBlockers(ctx context.Context, itemID string) ([]string, error) {
	return s.store.ListBlockers(ctx, itemID)
}

// wouldCycle reports whether adding edge (from → to) would create a cycle in the graph.
// DFS from 'to' following existing edges; if we can reach 'from', adding the edge would cycle.
func wouldCycle(edges [][2]string, from, to string) bool {
	adj := make(map[string][]string)
	for _, e := range edges {
		adj[e[0]] = append(adj[e[0]], e[1])
	}

	visited := make(map[string]bool)
	stack := []string{to}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == from {
			return true
		}
		if visited[n] {
			continue
		}
		visited[n] = true
		stack = append(stack, adj[n]...)
	}
	return false
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

// DeleteItem removes an item by ID.
func (s *HoneycombService) DeleteItem(ctx context.Context, id string) error {
	return s.store.DeleteItem(ctx, id)
}

// Prune delegates to the store's Prune implementation.
func (s *HoneycombService) Prune(ctx context.Context, opts hc.PruneOpts) (int, error) {
	return s.store.Prune(ctx, opts)
}
