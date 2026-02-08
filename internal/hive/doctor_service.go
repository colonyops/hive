package hive

import (
	"context"

	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/doctor"
	"github.com/hay-kot/hive/internal/core/session"
)

// DoctorService runs health checks on the hive setup.
type DoctorService struct {
	store  session.Store
	config *config.Config
}

// NewDoctorService creates a new DoctorService.
func NewDoctorService(store session.Store, cfg *config.Config) *DoctorService {
	return &DoctorService{
		store:  store,
		config: cfg,
	}
}

// RunChecks executes all doctor checks and returns results.
func (d *DoctorService) RunChecks(ctx context.Context, configPath string, autofix bool) []doctor.Result {
	checks := []doctor.Check{
		doctor.NewConfigCheck(d.config, configPath),
		doctor.NewOrphanCheck(d.store, d.config.ReposDir(), autofix),
	}
	return doctor.RunAll(ctx, checks)
}
