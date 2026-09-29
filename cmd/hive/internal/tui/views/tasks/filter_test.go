package tasks

import (
	"testing"

	"github.com/colonyops/hive/internal/core/hc"
)

func TestFilterItems(t *testing.T) {
	items := []hc.Item{
		{ID: "1", Status: hc.StatusOpen},
		{ID: "2", Status: hc.StatusInProgress},
		{ID: "3", Status: hc.StatusDone},
		{ID: "4", Status: hc.StatusCancelled},
		{ID: "5", Status: hc.StatusOpen},
	}

	tests := []struct {
		name    string
		filter  StatusFilter
		wantIDs []string
	}{
		{
			name:    "FilterAll returns all items",
			filter:  FilterAll,
			wantIDs: []string{"1", "2", "3", "4", "5"},
		},
		{
			name:    "FilterOpen returns open and in_progress",
			filter:  FilterOpen,
			wantIDs: []string{"1", "2", "5"},
		},
		{
			name:    "FilterActive returns in_progress only",
			filter:  FilterActive,
			wantIDs: []string{"2"},
		},
		{
			name:    "FilterDone returns done and cancelled",
			filter:  FilterDone,
			wantIDs: []string{"3", "4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterItems(items, tt.filter)
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("got %d items, want %d", len(got), len(tt.wantIDs))
			}
			for i, item := range got {
				if item.ID != tt.wantIDs[i] {
					t.Errorf("item[%d].ID = %q, want %q", i, item.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestFilterItems_Empty(t *testing.T) {
	got := filterItems(nil, FilterOpen)
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %d items", len(got))
	}

	got = filterItems([]hc.Item{}, FilterAll)
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %d items", len(got))
	}
}

func TestStatusFilter_String(t *testing.T) {
	tests := []struct {
		filter StatusFilter
		want   string
	}{
		{FilterAll, "All"},
		{FilterOpen, "Open"},
		{FilterActive, "Active"},
		{FilterDone, "Done"},
		{StatusFilter(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.filter.String(); got != tt.want {
			t.Errorf("StatusFilter(%d).String() = %q, want %q", tt.filter, got, tt.want)
		}
	}
}
