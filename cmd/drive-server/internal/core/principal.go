package core

import "github.com/colonyops/hive/cmd/drive-server/internal/config"

type Principal struct {
	UserID string
	Email  string
	// TokenName is the API token's name, or empty for a browser session.
	TokenName  string
	Scope      Scope
	Policy     config.Policy
	ViaSession bool
}
