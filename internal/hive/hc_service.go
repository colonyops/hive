package hive

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/hay-kot/criterio"
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

// ValidateCreateInput applies every rule CreateBulk enforces before it touches
// the store: the root is an epic, refs are unique, every blocker names a ref
// in the tree, and the blocker graph has no cycle. Errors carry criterio paths
// ("type", "children[2].ref", "children[1].blockers[0]") so a caller can show
// an agent exactly where its document is wrong.
func ValidateCreateInput(input hc.CreateInput) error {
	var errs criterio.FieldErrorsBuilder
	if input.Type != hc.ItemTypeEpic {
		errs = errs.Append("type", fmt.Errorf("root item must be of type epic, got %q", input.Type))
	}

	declaredAt := make(map[string]string)
	for path, node := range walkCreateInput(input, "") {
		if node.Ref == "" {
			continue
		}
		if prev, dup := declaredAt[node.Ref]; dup {
			errs = errs.Append(fieldPath(path, "ref"), fmt.Errorf("duplicate ref %q (first declared at %s)", node.Ref, prev))
			continue
		}
		declaredAt[node.Ref] = path
	}

	// A node without a ref cannot be named by anyone, so its path stands in as
	// its identity in the edge list.
	var edges [][2]string
	for path, node := range walkCreateInput(input, "") {
		self := node.Ref
		if self == "" {
			self = path
		}
		for i, blocker := range node.Blockers {
			field := fieldPath(path, fmt.Sprintf("blockers[%d]", i))
			if _, ok := declaredAt[blocker]; !ok {
				errs = errs.Append(field, fmt.Errorf("unknown blocker ref %q", blocker))
				continue
			}
			if hc.WouldCycle(edges, blocker, self) {
				errs = errs.Append(field, fmt.Errorf("blocker %q would create a cycle", blocker))
				continue
			}
			edges = append(edges, [2]string{blocker, self})
		}
	}
	return errs.ToError()
}

// walkCreateInput yields every node with its criterio path, depth first: ""
// for the root, then "children[0]", "children[0].children[1]", and so on.
func walkCreateInput(input hc.CreateInput, path string) iter.Seq2[string, hc.CreateInput] {
	return func(yield func(string, hc.CreateInput) bool) {
		if !yield(path, input) {
			return
		}
		for i, child := range input.Children {
			childPath := fieldPath(path, fmt.Sprintf("children[%d]", i))
			for p, n := range walkCreateInput(child, childPath) {
				if !yield(p, n) {
					return
				}
			}
		}
	}
}

func fieldPath(path, field string) string {
	if path == "" {
		return field
	}
	return path + "." + field
}

// blockerEdges maps each blocker ref to the generated id of the node that
// declared it. ValidateCreateInput has already proven every ref resolves.
func blockerEdges(entries []createInputEntry) [][2]string {
	refToID := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.input.Ref != "" {
			refToID[e.input.Ref] = e.item.ID
		}
	}
	var edges [][2]string
	for _, e := range entries {
		for _, blocker := range e.input.Blockers {
			edges = append(edges, [2]string{refToID[blocker], e.item.ID})
		}
	}
	return edges
}

// CreateBulk validates a CreateInput tree, walks it (BFS), and persists all
// items in one atomic call.
func (s *HoneycombService) CreateBulk(ctx context.Context, repoKey string, input hc.CreateInput) ([]hc.Item, error) {
	if err := ValidateCreateInput(input); err != nil {
		return nil, err
	}

	now := time.Now()
	var entries []createInputEntry
	for entry := range walkCreateInputWithEntry(input, repoKey, "", "", 0, now) {
		entries = append(entries, entry)
	}
	edges := blockerEdges(entries)

	items := make([]hc.Item, len(entries))
	for i, e := range entries {
		items[i] = e.item
	}

	if err := s.store.CreateBulkWithEdges(ctx, items, edges); err != nil {
		return nil, fmt.Errorf("bulk create items: %w", err)
	}

	return items, nil
}

// AddBlocker records that blockerID blocks blockedID.
// Cycle detection and the insert are performed atomically by the store.
// Returns hc.ErrCyclicDependency if the edge would create a cycle.
func (s *HoneycombService) AddBlocker(ctx context.Context, blockerID, blockedID string) error {
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

// GetItem returns an item by ID.
func (s *HoneycombService) GetItem(ctx context.Context, id string) (hc.Item, error) {
	return s.store.GetItem(ctx, id)
}

// UpdateItem applies a partial update to an item and returns the result.
// If the item is an epic and the update transitions it to a terminal status
// (done or cancelled) from a different status, all non-terminal descendants
// are updated to the same terminal status.
func (s *HoneycombService) UpdateItem(ctx context.Context, id string, update hc.ItemUpdate) (hc.Item, error) {
	old, err := s.store.GetItem(ctx, id)
	if err != nil {
		return hc.Item{}, fmt.Errorf("get item before update %q: %w", id, err)
	}

	updated, err := s.store.UpdateItem(ctx, id, update)
	if err != nil {
		return hc.Item{}, err
	}

	isTerminal := updated.Status == hc.StatusDone || updated.Status == hc.StatusCancelled
	statusChanged := old.Status != updated.Status
	if updated.IsEpic() && isTerminal && statusChanged {
		if cascadeErr := s.store.BulkUpdateStatus(ctx, updated.ID, updated.Status); cascadeErr != nil {
			return hc.Item{}, fmt.Errorf("cascade status to descendants of epic %q: %w", updated.ID, cascadeErr)
		}
	}

	return updated, nil
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
