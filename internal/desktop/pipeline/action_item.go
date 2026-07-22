package pipeline

import (
	"encoding/json"
	"fmt"

	"github.com/colonyops/hive/internal/desktop/feed"
)

// DecodeGitHubActionItem is the GitHub adapter seam from a persisted inbox
// payload to the action executor's GitHub item shape. The desktop service
// deliberately never accepts this shape from a client.
type DecodedActionItem struct {
	ID      string
	Kind    string
	Payload []byte
}

func DecodeGitHubActionItem(payload []byte, externalID string) (DecodedActionItem, error) {
	var item feed.Item
	if err := json.Unmarshal(payload, &item); err != nil {
		return DecodedActionItem{}, err
	}
	if item.ID == "" {
		item.ID = externalID
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		return DecodedActionItem{}, fmt.Errorf("encoding action item: %w", err)
	}
	return DecodedActionItem{ID: item.ID, Kind: item.Kind, Payload: encoded}, nil
}
