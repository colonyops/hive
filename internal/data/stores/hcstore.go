package stores

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/data/db"
)

// HCStore implements hc.Store using SQLite.
type HCStore struct {
	db *db.DB
}

var _ hc.Store = (*HCStore)(nil)

// NewHCStore creates a new SQLite-backed HC store.
func NewHCStore(database *db.DB) *HCStore {
	return &HCStore{db: database}
}

// CreateItems persists one or more HC items atomically.
func (s *HCStore) CreateItems(ctx context.Context, items []hc.Item) error {
	for _, item := range items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("validate hc item %q: %w", item.ID, err)
		}
	}

	return s.db.WithTx(ctx, func(q *db.Queries) error {
		for _, item := range items {
			err := q.CreateHCItem(ctx, db.CreateHCItemParams{
				ID:        item.ID,
				RepoKey:   item.RepoKey,
				EpicID:    item.EpicID,
				ParentID:  item.ParentID,
				SessionID: item.SessionID,
				Title:     item.Title,
				Desc:      item.Desc,
				Type:      item.Type,
				Status:    item.Status,
				Depth:     int64(item.Depth),
				CreatedAt: item.CreatedAt.UnixNano(),
				UpdatedAt: item.UpdatedAt.UnixNano(),
			})
			if err != nil {
				return fmt.Errorf("create hc item %q: %w", item.ID, err)
			}
		}
		return nil
	})
}

// GetItem retrieves a single HC item by ID.
func (s *HCStore) GetItem(ctx context.Context, id string) (hc.Item, error) {
	row, err := s.db.Queries().GetHCItem(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return hc.Item{}, fmt.Errorf("hc item %q: %w", id, hc.ErrNotFound)
	}
	if err != nil {
		return hc.Item{}, fmt.Errorf("get hc item: %w", err)
	}
	item, err := s.fetchHCItem(ctx, row)
	if err != nil {
		return hc.Item{}, err
	}
	return item, nil
}

// UpdateItem applies partial updates to an HC item.
func (s *HCStore) UpdateItem(ctx context.Context, id string, update hc.ItemUpdate) (hc.Item, error) {
	var updated db.HcItem
	err := s.db.WithTx(ctx, func(q *db.Queries) error {
		existing, getErr := q.GetHCItem(ctx, id)
		if errors.Is(getErr, sql.ErrNoRows) {
			return fmt.Errorf("hc item %q: %w", id, hc.ErrNotFound)
		}
		if getErr != nil {
			return fmt.Errorf("get hc item for update: %w", getErr)
		}

		newStatus := existing.Status
		newSessionID := existing.SessionID
		if update.Status != nil {
			newStatus = *update.Status
		}
		if update.SessionID != nil {
			newSessionID = *update.SessionID
		}

		if err := q.UpdateHCItem(ctx, db.UpdateHCItemParams{
			Status:    newStatus,
			SessionID: newSessionID,
			UpdatedAt: time.Now().UnixNano(),
			ID:        id,
		}); err != nil {
			return fmt.Errorf("update hc item: %w", err)
		}

		var fetchErr error
		updated, fetchErr = q.GetHCItem(ctx, id)
		if fetchErr != nil {
			return fmt.Errorf("re-fetch after update: %w", fetchErr)
		}
		return nil
	})
	if err != nil {
		return hc.Item{}, err
	}

	item, err := s.fetchHCItem(ctx, updated)
	if err != nil {
		return hc.Item{}, err
	}
	return item, nil
}

// ListItems returns HC items matching the given filter.
func (s *HCStore) ListItems(ctx context.Context, filter hc.ListFilter) ([]hc.Item, error) {
	rows, err := s.listHCRows(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list hc items: %w", err)
	}

	blockedSet, err := s.fetchBlockedSet(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch blocked set: %w", err)
	}

	result := make([]hc.Item, 0, len(rows))
	for _, row := range rows {
		item := buildHCItem(row, blockedSet)
		if !filter.Matches(item) {
			continue
		}
		result = append(result, item)
	}

	return result, nil
}

// listHCRows selects the narrowest SQL query that can satisfy the list filter.
func (s *HCStore) listHCRows(ctx context.Context, filter hc.ListFilter) ([]db.HcItem, error) {
	switch {
	case filter.EpicID != "" && filter.Status != nil:
		return s.db.Queries().ListHCItemsByEpicAndStatus(ctx, db.ListHCItemsByEpicAndStatusParams{
			EpicID: filter.EpicID,
			Status: *filter.Status,
		})
	case filter.EpicID != "":
		return s.db.Queries().ListHCItemsByEpic(ctx, filter.EpicID)
	case filter.SessionID != "":
		return s.db.Queries().ListHCItemsBySession(ctx, filter.SessionID)
	case filter.RepoKey != "":
		return s.db.Queries().ListHCItemsByRepo(ctx, filter.RepoKey)
	case filter.Status != nil:
		return s.db.Queries().ListAllHCItemsByStatus(ctx, *filter.Status)
	default:
		return s.db.Queries().ListAllHCItems(ctx)
	}
}

// NextItem returns the next actionable item for the given filter.
// When filter.SessionID is set, it first tries to resume an in_progress task
// for that session before falling back to claiming an unassigned open task.
func (s *HCStore) NextItem(ctx context.Context, filter hc.NextFilter) (hc.Item, bool, error) {
	if filter.SessionID != "" {
		item, ok, err := s.resumeItemForSession(ctx, filter)
		if err != nil {
			return hc.Item{}, false, err
		}
		if ok {
			return item, true, nil
		}
	}

	// Fall back to claiming the next unassigned open task (session_id = "").
	if filter.EpicID != "" {
		row, err := s.db.Queries().NextHCItemForSessionInEpic(ctx, db.NextHCItemForSessionInEpicParams{
			SessionID: "",
			EpicID:    filter.EpicID,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return hc.Item{}, false, nil
		}
		if err != nil {
			return hc.Item{}, false, fmt.Errorf("next hc item for session in epic: %w", err)
		}
		item, err := s.fetchHCItem(ctx, row)
		if err != nil {
			return hc.Item{}, false, err
		}
		if filter.RepoKey != "" && item.RepoKey != filter.RepoKey {
			return hc.Item{}, false, nil
		}
		return item, true, nil
	}

	row, err := s.db.Queries().NextHCItemForSession(ctx, "")
	if errors.Is(err, sql.ErrNoRows) {
		return hc.Item{}, false, nil
	}
	if err != nil {
		return hc.Item{}, false, fmt.Errorf("next hc item for session: %w", err)
	}
	item, err := s.fetchHCItem(ctx, row)
	if err != nil {
		return hc.Item{}, false, err
	}
	if filter.RepoKey != "" && item.RepoKey != filter.RepoKey {
		return hc.Item{}, false, nil
	}
	return item, true, nil
}

// resumeItemForSession returns an in_progress leaf task assigned to the given session.
func (s *HCStore) resumeItemForSession(ctx context.Context, filter hc.NextFilter) (hc.Item, bool, error) {
	if filter.EpicID != "" {
		row, err := s.db.Queries().ResumeHCItemForSessionInEpic(ctx, db.ResumeHCItemForSessionInEpicParams{
			SessionID: filter.SessionID,
			EpicID:    filter.EpicID,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return hc.Item{}, false, nil
		}
		if err != nil {
			return hc.Item{}, false, fmt.Errorf("resume hc item for session in epic: %w", err)
		}
		item, err := s.fetchHCItem(ctx, row)
		if err != nil {
			return hc.Item{}, false, err
		}
		return item, true, nil
	}

	row, err := s.db.Queries().ResumeHCItemForSession(ctx, filter.SessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return hc.Item{}, false, nil
	}
	if err != nil {
		return hc.Item{}, false, fmt.Errorf("resume hc item for session: %w", err)
	}
	item, err := s.fetchHCItem(ctx, row)
	if err != nil {
		return hc.Item{}, false, err
	}
	return item, true, nil
}

// DeleteItem removes an HC item by ID.
func (s *HCStore) DeleteItem(ctx context.Context, id string) error {
	if err := s.db.Queries().DeleteHCItem(ctx, id); err != nil {
		return fmt.Errorf("delete hc item: %w", err)
	}
	return nil
}

// AddComment records a comment on an HC item.
func (s *HCStore) AddComment(ctx context.Context, c hc.Comment) error {
	_, err := s.db.Queries().InsertHCComment(ctx, db.InsertHCCommentParams{
		ID:        c.ID,
		ItemID:    c.ItemID,
		Message:   c.Message,
		CreatedAt: c.CreatedAt.UnixNano(),
	})
	if err != nil {
		return fmt.Errorf("add hc comment: %w", err)
	}
	return nil
}

// ListComments returns all comments for the given item in chronological order.
func (s *HCStore) ListComments(ctx context.Context, itemID string) ([]hc.Comment, error) {
	rows, err := s.db.Queries().ListHCComments(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("list hc comments: %w", err)
	}

	result := make([]hc.Comment, 0, len(rows))
	for _, row := range rows {
		result = append(result, rowToHCComment(row))
	}
	return result, nil
}

// ListRepoKeys returns all distinct, non-empty repo keys in sorted order.
func (s *HCStore) ListRepoKeys(ctx context.Context) ([]string, error) {
	keys, err := s.db.Queries().ListHCRepoKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list hc repo keys: %w", err)
	}
	return keys, nil
}

// Prune removes old done/cancelled items and their comments.
func (s *HCStore) Prune(ctx context.Context, opts hc.PruneOpts) (int, error) {
	allRows, err := s.db.Queries().ListAllHCItems(ctx)
	if err != nil {
		return 0, fmt.Errorf("list hc items for prune: %w", err)
	}
	return s.prune(ctx, allRows, opts)
}

// prune is the internal implementation separated for testability.
func (s *HCStore) prune(ctx context.Context, allRows []db.HcItem, opts hc.PruneOpts) (int, error) {
	cutoff := time.Now().Add(-opts.OlderThan).UnixNano()

	statuses := opts.Statuses
	if len(statuses) == 0 {
		statuses = []hc.Status{hc.StatusDone, hc.StatusCancelled}
	}

	statusSet := make(map[hc.Status]struct{}, len(statuses))
	for _, status := range statuses {
		statusSet[status] = struct{}{}
	}

	// Scope to repo when requested.
	scopedRows := allRows
	if opts.RepoKey != "" {
		scopedRows = make([]db.HcItem, 0, len(allRows))
		for _, row := range allRows {
			if row.RepoKey == opts.RepoKey {
				scopedRows = append(scopedRows, row)
			}
		}
	}

	childrenByParent := make(map[string][]string, len(scopedRows))
	depthByID := make(map[string]int64, len(scopedRows))
	pruneRoots := make([]string, 0)
	for _, row := range scopedRows {
		depthByID[row.ID] = row.Depth
		if row.ParentID != "" {
			childrenByParent[row.ParentID] = append(childrenByParent[row.ParentID], row.ID)
		}

		if _, ok := statusSet[row.Status]; !ok {
			continue
		}
		if row.UpdatedAt >= cutoff {
			continue
		}

		pruneRoots = append(pruneRoots, row.ID)
	}

	pruneIDs := collectHCSubtreeIDs(pruneRoots, childrenByParent)
	total := len(pruneIDs)

	if opts.DryRun {
		return total, nil
	}

	err := s.db.WithTx(ctx, func(q *db.Queries) error {
		idsByDepthDesc := orderHCIDsByDepthDesc(pruneIDs, depthByID)
		for _, id := range idsByDepthDesc {
			if txErr := q.DeleteHCCommentsByItemID(ctx, id); txErr != nil {
				return fmt.Errorf("delete hc comments for %q: %w", id, txErr)
			}
			if txErr := q.DeleteHCItem(ctx, id); txErr != nil {
				return fmt.Errorf("delete hc item %q: %w", id, txErr)
			}
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	return total, nil
}

// collectHCSubtreeIDs expands root IDs into a de-duplicated set that includes all descendants.
func collectHCSubtreeIDs(roots []string, childrenByParent map[string][]string) map[string]struct{} {
	pruneIDs := make(map[string]struct{}, len(roots))
	stack := make([]string, len(roots))
	copy(stack, roots)

	for len(stack) > 0 {
		last := len(stack) - 1
		id := stack[last]
		stack = stack[:last]

		if _, seen := pruneIDs[id]; seen {
			continue
		}
		pruneIDs[id] = struct{}{}

		children := childrenByParent[id]
		stack = append(stack, children...)
	}

	return pruneIDs
}

// orderHCIDsByDepthDesc returns IDs ordered deepest-first for safe child-before-parent deletes.
func orderHCIDsByDepthDesc(ids map[string]struct{}, depthByID map[string]int64) []string {
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}

	sort.SliceStable(ordered, func(i, j int) bool {
		return depthByID[ordered[i]] > depthByID[ordered[j]]
	})

	return ordered
}

// fetchBlockedSet queries all parent IDs with open/in_progress children, plus
// items that have explicit open/in_progress blockers.
func (s *HCStore) fetchBlockedSet(ctx context.Context) (map[string]struct{}, error) {
	// Parent IDs with open/in_progress children (hierarchy blocking)
	parentIDs, err := s.db.Queries().ListHCBlockedParentIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list hc blocked parent ids: %w", err)
	}
	// Items with open/in_progress explicit blockers
	explicitIDs, err := s.db.Queries().ListHCExplicitlyBlockedIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list hc explicitly blocked ids: %w", err)
	}
	set := make(map[string]struct{}, len(parentIDs)+len(explicitIDs))
	for _, id := range parentIDs {
		set[id] = struct{}{}
	}
	for _, id := range explicitIDs {
		set[id] = struct{}{}
	}
	return set, nil
}

// buildHCItemWithBlockers converts a database row to hc.Item.
// blockedSet contains IDs that are blocked (by children or explicit blockers).
// blockerIDsMap maps item ID to its explicit open blocker IDs.
func buildHCItemWithBlockers(row db.HcItem, blockedSet map[string]struct{}, blockerIDsMap map[string][]string) hc.Item {
	_, blocked := blockedSet[row.ID]
	return hc.Item{
		ID:         row.ID,
		RepoKey:    row.RepoKey,
		EpicID:     row.EpicID,
		ParentID:   row.ParentID,
		SessionID:  row.SessionID,
		Title:      row.Title,
		Desc:       row.Desc,
		Type:       row.Type,
		Status:     row.Status,
		Blocked:    blocked,
		BlockerIDs: blockerIDsMap[row.ID],
		Depth:      int(row.Depth),
		CreatedAt:  time.Unix(0, row.CreatedAt),
		UpdatedAt:  time.Unix(0, row.UpdatedAt),
	}
}

// buildHCItem converts a database row to hc.Item using the pre-fetched blocked set.
func buildHCItem(row db.HcItem, blockedSet map[string]struct{}) hc.Item {
	return buildHCItemWithBlockers(row, blockedSet, map[string][]string{})
}

// fetchHCItem converts a single database row to hc.Item, issuing DB calls to compute Blocked and BlockerIDs.
func (s *HCStore) fetchHCItem(ctx context.Context, row db.HcItem) (hc.Item, error) {
	count, err := s.db.Queries().CountHCOpenChildren(ctx, row.ID)
	if err != nil {
		return hc.Item{}, fmt.Errorf("count open children for %q: %w", row.ID, err)
	}

	blockerIDs, err := s.db.Queries().ListHCOpenBlockerIDsForItem(ctx, row.ID)
	if err != nil {
		return hc.Item{}, fmt.Errorf("list open blockers for %q: %w", row.ID, err)
	}

	blockedSet := map[string]struct{}{}
	if count > 0 || len(blockerIDs) > 0 {
		blockedSet[row.ID] = struct{}{}
	}

	blockerIDsMap := map[string][]string{}
	if len(blockerIDs) > 0 {
		blockerIDsMap[row.ID] = blockerIDs
	}

	return buildHCItemWithBlockers(row, blockedSet, blockerIDsMap), nil
}

// CreateBulkWithEdges creates items and wires their explicit blocker edges atomically.
// Cycle validation must be done by the caller; this method only enforces FK constraints.
func (s *HCStore) CreateBulkWithEdges(ctx context.Context, items []hc.Item, edges [][2]string) error {
	for _, item := range items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("validate hc item %q: %w", item.ID, err)
		}
	}
	return s.db.WithTx(ctx, func(q *db.Queries) error {
		for _, item := range items {
			if err := q.CreateHCItem(ctx, db.CreateHCItemParams{
				ID:        item.ID,
				RepoKey:   item.RepoKey,
				EpicID:    item.EpicID,
				ParentID:  item.ParentID,
				SessionID: item.SessionID,
				Title:     item.Title,
				Desc:      item.Desc,
				Type:      item.Type,
				Status:    item.Status,
				Depth:     int64(item.Depth),
				CreatedAt: item.CreatedAt.UnixNano(),
				UpdatedAt: item.UpdatedAt.UnixNano(),
			}); err != nil {
				return fmt.Errorf("create hc item %q: %w", item.ID, err)
			}
		}
		for _, edge := range edges {
			if err := q.AddHCBlocker(ctx, db.AddHCBlockerParams{
				BlockerID: edge[0],
				BlockedID: edge[1],
			}); err != nil {
				return fmt.Errorf("wire blocker edge %q→%q: %w", edge[0], edge[1], err)
			}
		}
		return nil
	})
}

// AddBlocker records that blockerID blocks blockedID. Cycle detection and the
// INSERT are performed inside a single transaction so no partial state is possible.
// Returns ErrCyclicDependency if the edge would create a cycle.
func (s *HCStore) AddBlocker(ctx context.Context, blockerID, blockedID string) error {
	return s.db.WithTx(ctx, func(q *db.Queries) error {
		if _, err := q.GetHCItem(ctx, blockerID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("blocker item %q: %w", blockerID, hc.ErrNotFound)
			}
			return fmt.Errorf("get blocker item %q: %w", blockerID, err)
		}
		if _, err := q.GetHCItem(ctx, blockedID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("blocked item %q: %w", blockedID, hc.ErrNotFound)
			}
			return fmt.Errorf("get blocked item %q: %w", blockedID, err)
		}

		rows, err := q.ListAllHCBlockerEdges(ctx)
		if err != nil {
			return fmt.Errorf("list blocker edges: %w", err)
		}
		existing := make([][2]string, len(rows))
		for i, row := range rows {
			existing[i] = [2]string{row.BlockerID, row.BlockedID}
		}
		if hc.WouldCycle(existing, blockerID, blockedID) {
			return hc.ErrCyclicDependency
		}

		if err := q.AddHCBlocker(ctx, db.AddHCBlockerParams{
			BlockerID: blockerID,
			BlockedID: blockedID,
		}); err != nil {
			return fmt.Errorf("add hc blocker: %w", err)
		}
		return nil
	})
}

// RemoveBlocker removes the explicit blocker relationship.
func (s *HCStore) RemoveBlocker(ctx context.Context, blockerID, blockedID string) error {
	if err := s.db.Queries().RemoveHCBlocker(ctx, db.RemoveHCBlockerParams{
		BlockerID: blockerID,
		BlockedID: blockedID,
	}); err != nil {
		return fmt.Errorf("remove hc blocker: %w", err)
	}
	return nil
}

// ListBlockers returns IDs of open/in_progress items that explicitly block the given item.
func (s *HCStore) ListBlockers(ctx context.Context, itemID string) ([]string, error) {
	ids, err := s.db.Queries().ListHCOpenBlockerIDsForItem(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("list hc open blockers for %q: %w", itemID, err)
	}
	return ids, nil
}

// ListBlockerEdges returns all blocker edges as [blockerID, blockedID] pairs.
func (s *HCStore) ListBlockerEdges(ctx context.Context) ([][2]string, error) {
	rows, err := s.db.Queries().ListAllHCBlockerEdges(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all hc blocker edges: %w", err)
	}
	edges := make([][2]string, len(rows))
	for i, row := range rows {
		edges[i] = [2]string{row.BlockerID, row.BlockedID}
	}
	return edges, nil
}

// rowToHCComment maps a database comment row to a domain comment.
func rowToHCComment(row db.HcComment) hc.Comment {
	return hc.Comment{
		ID:        row.ID,
		ItemID:    row.ItemID,
		Message:   row.Message,
		CreatedAt: time.Unix(0, row.CreatedAt),
	}
}
