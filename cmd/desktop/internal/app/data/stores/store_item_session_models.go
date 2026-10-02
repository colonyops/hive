package stores

import "github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"

// ItemSession is one recorded hive session created on an inbox item's
// behalf.
type ItemSession struct {
	SessionID   string `json:"sessionId"`
	ProfileID   string `json:"profileId"`
	SourceKind  string `json:"sourceKind"`
	SourceScope string `json:"sourceScope"`
	ExternalID  string `json:"externalId"`
	CreatedAt   int64  `json:"createdAt"`
}

func mapItemSessionFromDB(row queries.ItemSession) ItemSession {
	return ItemSession{
		SessionID:   row.SessionID,
		ProfileID:   row.ProfileID,
		SourceKind:  row.SourceKind,
		SourceScope: row.SourceScope,
		ExternalID:  row.ExternalID,
		CreatedAt:   row.CreatedAt,
	}
}

// ItemChat is one agent workspace chat opened on an inbox item's behalf, with
// the chat's current name and workspace.
type ItemChat struct {
	ChatID    int64  `json:"chatId"`
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
}

func mapItemChatFromDB(row queries.ListItemChatsRow) ItemChat {
	return ItemChat{ChatID: row.ChatID, Workspace: row.Workspace, Name: row.Name, CreatedAt: row.CreatedAt}
}
