package stores

import "github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"

type OrchestratorToken struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Hint is the token's last four characters.
	Hint       string `json:"hint"`
	CreatedAt  int64  `json:"createdAt"`
	LastUsedAt int64  `json:"lastUsedAt"`
}

func mapOrchestratorTokenFromDB(row queries.OrchestratorToken) OrchestratorToken {
	return OrchestratorToken{
		ID: row.ID, Name: row.Name, Hint: row.Hint, CreatedAt: row.CreatedAt,
		LastUsedAt: row.LastUsedAt.Int64,
	}
}
