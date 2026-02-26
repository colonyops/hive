package hc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestItemValidate(t *testing.T) {
	validEpic := Item{
		ID:     "hc-abc123",
		Title:  "My Epic",
		Type:   ItemTypeEpic,
		Status: StatusOpen,
		EpicID: "",
		Depth:  0,
	}

	tests := []struct {
		name    string
		item    Item
		wantErr bool
	}{
		{
			name:    "missing ID",
			item:    func() Item { i := validEpic; i.ID = ""; return i }(),
			wantErr: true,
		},
		{
			name:    "missing title",
			item:    func() Item { i := validEpic; i.Title = ""; return i }(),
			wantErr: true,
		},
		{
			name:    "invalid status",
			item:    func() Item { i := validEpic; i.Status = Status("bad"); return i }(),
			wantErr: true,
		},
		{
			name:    "invalid type",
			item:    func() Item { i := validEpic; i.Type = ItemType("bad"); return i }(),
			wantErr: true,
		},
		{
			name:    "depth negative",
			item:    func() Item { i := validEpic; i.Depth = -1; return i }(),
			wantErr: true,
		},
		{
			name:    "depth too deep",
			item:    func() Item { i := validEpic; i.Depth = 11; return i }(),
			wantErr: true,
		},
		{
			name: "epic with parent",
			item: Item{
				ID:     "hc-abc123",
				Title:  "My Epic",
				Type:   ItemTypeEpic,
				Status: StatusOpen,
				EpicID: "hc-parent",
				Depth:  0,
			},
			wantErr: true,
		},
		{
			name: "non-epic with empty epic_id",
			item: Item{
				ID:     "hc-abc123",
				Title:  "My Task",
				Type:   ItemTypeTask,
				Status: StatusOpen,
				EpicID: "",
				Depth:  1,
			},
			wantErr: true,
		},
		{
			name:    "valid epic",
			item:    validEpic,
			wantErr: false,
		},
		{
			name: "valid task",
			item: Item{
				ID:     "hc-task1",
				Title:  "My Task",
				Type:   ItemTypeTask,
				Status: StatusOpen,
				EpicID: "epic-1",
				Depth:  1,
			},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.item.Validate()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
