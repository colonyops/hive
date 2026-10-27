package core

import "github.com/colonyops/hive/cmd/drive-server/internal/config"

type Principal struct {
	UserID     string
	Email      string
	Scope      Scope
	Policy     config.Policy
	ViaSession bool
}
