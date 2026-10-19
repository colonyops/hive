package app

import (
	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/pkg/executil"
)

// ReloadConfig replaces the CLI and engine settings after validating the whole file.
// The TUI serializes reload requests and adopts the new services on completion.
func (a *App) ReloadConfig(path string) error {
	cfg, err := config.LoadReload(path, a.Config)
	if err != nil {
		return err
	}
	if err := a.Reload(&cfg.Config); err != nil {
		return err
	}
	a.Config = cfg
	a.CommandSet.SetUser(cfg.UserCommands)
	a.Doctor = a.Engine.Doctor(cfg, a.pluginInfos)
	a.Sources = BuildSourceRegistry(a.Logger, cfg, &executil.RealExecutor{}, a.KV())
	return nil
}
