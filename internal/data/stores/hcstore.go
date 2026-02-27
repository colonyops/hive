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
	"github.com/colonyops/hive/pkg/randid"
	"github.com/rs/zerolog/log"
)

var (
	fallbackHCStatus       = hc.StatusOpen
	fallbackHCActivityType = hc.ActivityTypeUpdate
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

// CreateItem persists a new HC item.
func (s *HCStore) CreateItem(ctx context.Context, item hc.Item) error {
	if err := item.Validate(); err != nil {
		return fmt.Errorf("validate hc item %q: %w", item.ID, err)
	}

	err := s.db.Queries().CreateHCItem(ctx, db.CreateHCItemParams{
		ID:        item.ID,
		RepoKey:   item.RepoKey,
		EpicID:    item.EpicID,
		ParentID:  item.ParentID,
		SessionID: item.SessionID,
		Title:     item.Title,
		Desc:      item.Desc,
		Type:      string(item.Type),
		Status:    string(item.Status),
		Depth:     int64(item.Depth),
		CreatedAt: item.CreatedAt.UnixNano(),
		UpdatedAt: item.UpdatedAt.UnixNano(),
	})
	if err != nil {
		return fmt.Errorf("create hc item: %w", err)
	}

	return nil
}

// CreateItemBatch persists multiple HC items atomically.
func (s *HCStore) CreateItemBatch(ctx context.Context, items []hc.Item) error {
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
				Type:      string(item.Type),
				Status:    string(item.Status),
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
	if err != nil {
		return hc.Item{}, fmt.Errorf("get hc item: %w", err)
	}
	return s.rowToHCItem(ctx, row), nil
}

// UpdateItem applies partial updates to an HC item, atomically logging a status_change activity if status changed.
func (s *HCStore) UpdateItem(ctx context.Context, id string, update hc.ItemUpdate) (hc.Item, error) {
	var updated db.HcItem
	err := s.db.WithTx(ctx, func(q *db.Queries) error {
		existing, getErr := q.GetHCItem(ctx, id)
		if getErr != nil {
			return fmt.Errorf("get hc item for update: %w", getErr)
		}

		newStatus := existing.Status
		newSessionID := existing.SessionID
		if update.Status != nil {
			newStatus = string(*update.Status)
		}
		if update.SessionID != nil {
			newSessionID = *update.SessionID
		}

		now := time.Now()
		statusChanged := newStatus != existing.Status

		if err := q.UpdateHCItem(ctx, db.UpdateHCItemParams{
			Status:    newStatus,
			SessionID: newSessionID,
			UpdatedAt: now.UnixNano(),
			ID:        id,
		}); err != nil {
			return fmt.Errorf("update hc item: %w", err)
		}

		if statusChanged {
			activityID := generateHCID()
			msg := fmt.Sprintf("status changed from %s to %s", existing.Status, newStatus)
			if _, err := q.InsertHCActivity(ctx, db.InsertHCActivityParams{
				ID:        activityID,
				ItemID:    id,
				Type:      string(hc.ActivityTypeStatusChange),
				Message:   msg,
				CreatedAt: now.UnixNano(),
			}); err != nil {
				return fmt.Errorf("log status_change activity: %w", err)
			}
		}

		var fetchErr error
		updated, fetchErr = q.GetHCItem(ctx, id)
		return fetchErr
	})
	if err != nil {
		return hc.Item{}, err
	}

	return s.rowToHCItem(ctx, updated), nil
}

// ListItems returns HC items matching the given filter.
func (s *HCStore) ListItems(ctx context.Context, filter hc.ListFilter) ([]hc.Item, error) {
	rows, err := s.listHCRows(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list hc items: %w", err)
	}

	result := make([]hc.Item, 0, len(rows))
	for _, row := range rows {
		item := s.rowToHCItem(ctx, row)
		if !matchesHCListFilter(item, filter) {
			continue
		}
		result = append(result, item)
	}

	return result, nil
}

func (s *HCStore) listHCRows(ctx context.Context, filter hc.ListFilter) ([]db.HcItem, error) {
	switch {
	case filter.EpicID != "" && filter.Status != nil:
		return s.db.Queries().ListHCItemsByEpicAndStatus(ctx, db.ListHCItemsByEpicAndStatusParams{
			EpicID: filter.EpicID,
			Status: string(*filter.Status),
		})
	case filter.EpicID != "":
		return s.db.Queries().ListHCItemsByEpic(ctx, filter.EpicID)
	case filter.SessionID != "":
		return s.db.Queries().ListHCItemsBySession(ctx, filter.SessionID)
	case filter.RepoKey != "":
		return s.db.Queries().ListHCItemsByRepo(ctx, filter.RepoKey)
	case filter.Status != nil:
		return s.db.Queries().ListAllHCItemsByStatus(ctx, string(*filter.Status))
	default:
		return s.db.Queries().ListAllHCItems(ctx)
	}
}

func matchesHCListFilter(item hc.Item, filter hc.ListFilter) bool {
	if filter.RepoKey != "" && item.RepoKey != filter.RepoKey {
		return false
	}
	if filter.SessionID != "" && item.SessionID != filter.SessionID {
		return false
	}
	if filter.Status != nil && item.Status != *filter.Status {
		return false
	}
	return true
}

// NextItem returns the next actionable item for the given filter.
func (s *HCStore) NextItem(ctx context.Context, filter hc.NextFilter) (hc.Item, bool, error) {
	if filter.EpicID != "" {
		row, err := s.db.Queries().NextHCItemForSessionInEpic(ctx, db.NextHCItemForSessionInEpicParams{
			SessionID: filter.SessionID,
			EpicID:    filter.EpicID,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return hc.Item{}, false, nil
		}
		if err != nil {
			return hc.Item{}, false, fmt.Errorf("next hc item for session in epic: %w", err)
		}
		return s.rowToHCItem(ctx, row), true, nil
	}

	row, err := s.db.Queries().NextHCItemForSession(ctx, filter.SessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return hc.Item{}, false, nil
	}
	if err != nil {
		return hc.Item{}, false, fmt.Errorf("next hc item for session: %w", err)
	}
	return s.rowToHCItem(ctx, row), true, nil
}

// DeleteItem removes an HC item by ID.
func (s *HCStore) DeleteItem(ctx context.Context, id string) error {
	if err := s.db.Queries().DeleteHCItem(ctx, id); err != nil {
		return fmt.Errorf("delete hc item: %w", err)
	}
	return nil
}

// LogActivity records a discrete event against an HC item.
func (s *HCStore) LogActivity(ctx context.Context, a hc.Activity) error {
	_, err := s.db.Queries().InsertHCActivity(ctx, db.InsertHCActivityParams{
		ID:        a.ID,
		ItemID:    a.ItemID,
		Type:      string(a.Type),
		Message:   a.Message,
		CreatedAt: a.CreatedAt.UnixNano(),
	})
	if err != nil {
		return fmt.Errorf("log hc activity: %w", err)
	}
	return nil
}

// ListActivity returns all activity for the given item, ordered by creation time.
func (s *HCStore) ListActivity(ctx context.Context, itemID string) ([]hc.Activity, error) {
	rows, err := s.db.Queries().ListHCActivity(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("list hc activity: %w", err)
	}

	result := make([]hc.Activity, 0, len(rows))
	for _, row := range rows {
		result = append(result, rowToHCActivity(row))
	}
	return result, nil
}

// LatestCheckpoint returns the most recent checkpoint activity for the given item.
func (s *HCStore) LatestCheckpoint(ctx context.Context, itemID string) (hc.Activity, bool, error) {
	row, err := s.db.Queries().LatestHCCheckpoint(ctx, itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return hc.Activity{}, false, nil
	}
	if err != nil {
		return hc.Activity{}, false, fmt.Errorf("latest hc checkpoint: %w", err)
	}
	return rowToHCActivity(row), true, nil
}

// Prune removes old done/cancelled items and their activity.
func (s *HCStore) Prune(ctx context.Context, opts hc.PruneOpts) (int, error) {
	cutoff := time.Now().Add(-opts.OlderThan).UnixNano()

	statuses := opts.Statuses
	if len(statuses) == 0 {
		statuses = []hc.Status{hc.StatusDone, hc.StatusCancelled}
	}

	allRows, err := s.db.Queries().ListAllHCItems(ctx)
	if err != nil {
		return 0, fmt.Errorf("list hc items for prune: %w", err)
	}

	statusSet := make(map[hc.Status]struct{}, len(statuses))
	for _, status := range statuses {
		statusSet[status] = struct{}{}
	}

	childrenByParent := make(map[string][]string, len(allRows))
	depthByID := make(map[string]int64, len(allRows))
	pruneRoots := make([]string, 0)
	for _, row := range allRows {
		depthByID[row.ID] = row.Depth
		if row.ParentID != "" {
			childrenByParent[row.ParentID] = append(childrenByParent[row.ParentID], row.ID)
		}

		status, parseErr := hc.ParseStatus(row.Status)
		if parseErr != nil {
			continue
		}
		if _, ok := statusSet[status]; !ok {
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

	err = s.db.WithTx(ctx, func(q *db.Queries) error {
		for _, status := range statuses {
			if txErr := q.PruneHCActivityByStatus(ctx, db.PruneHCActivityByStatusParams{
				Status:    string(status),
				UpdatedAt: cutoff,
			}); txErr != nil {
				return fmt.Errorf("prune hc activity (%s): %w", status, txErr)
			}
		}

		idsByDepthDesc := orderHCIDsByDepthDesc(pruneIDs, depthByID)
		for _, id := range idsByDepthDesc {
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

func (s *HCStore) rowToHCItem(ctx context.Context, row db.HcItem) hc.Item {
	blocked := false
	count, err := s.db.Queries().CountHCOpenChildren(ctx, row.ID)
	if err != nil {
		log.Debug().Err(err).Str("id", row.ID).Msg("failed to count open children for hc item")
	} else {
		blocked = count > 0
	}

	return hc.Item{
		ID:        row.ID,
		RepoKey:   row.RepoKey,
		EpicID:    row.EpicID,
		ParentID:  row.ParentID,
		SessionID: row.SessionID,
		Title:     row.Title,
		Desc:      row.Desc,
		Type:      parseStoredHCItemType(row),
		Status:    parseStoredHCStatus(row),
		Blocked:   blocked,
		Depth:     int(row.Depth),
		CreatedAt: time.Unix(0, row.CreatedAt),
		UpdatedAt: time.Unix(0, row.UpdatedAt),
	}
}

func rowToHCActivity(row db.HcActivity) hc.Activity {
	actType := parseStoredHCActivityType(row)
	return hc.Activity{
		ID:        row.ID,
		ItemID:    row.ItemID,
		Type:      actType,
		Message:   row.Message,
		CreatedAt: time.Unix(0, row.CreatedAt),
	}
}

func parseStoredHCStatus(row db.HcItem) hc.Status {
	status, err := hc.ParseStatus(row.Status)
	if err != nil {
		log.Warn().Err(err).Str("id", row.ID).Str("status", row.Status).Msg("invalid status in stored hc item, defaulting to open")
		return fallbackHCStatus
	}
	return status
}

func parseStoredHCItemType(row db.HcItem) hc.ItemType {
	itemType, err := hc.ParseItemType(row.Type)
	if err != nil {
		log.Warn().Err(err).Str("id", row.ID).Str("type", row.Type).Msg("invalid type in stored hc item, defaulting to task")
		return hc.ItemTypeTask
	}
	return itemType
}

func parseStoredHCActivityType(row db.HcActivity) hc.ActivityType {
	actType, err := hc.ParseActivityType(row.Type)
	if err != nil {
		log.Warn().Err(err).Str("id", row.ID).Str("type", row.Type).Msg("invalid type in stored hc activity, defaulting to update")
		return fallbackHCActivityType
	}
	return actType
}

func generateHCID() string {
	return "hc-" + randid.Generate(8)
}
