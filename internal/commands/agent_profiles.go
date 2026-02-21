package commands

import (
	"sort"

	"github.com/colonyops/hive/internal/core/config"
)

func agentProfileNames(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Agents.Profiles))
	for k := range cfg.Agents.Profiles {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
