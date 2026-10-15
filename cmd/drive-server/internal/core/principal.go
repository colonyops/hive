package core

import "github.com/colonyops/hive/cmd/drive-server/internal/config"

// Principal is who a request acts as, after authentication.
type Principal struct {
	UserID     string
	Email      string
	Scope      Scope
	Policy     config.Policy
	ViaSession bool
}
