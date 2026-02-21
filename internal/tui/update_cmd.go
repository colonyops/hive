package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	corekv "github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/hive/updatecheck"
	"github.com/rs/zerolog/log"
)

type updateAvailableMsg struct {
	result *updatecheck.Result
}

func checkForUpdate(kvStore corekv.KV, currentVersion string) tea.Cmd {
	return func() tea.Msg {
		result, err := updatecheck.Check(context.Background(), kvStore, currentVersion)
		if err != nil {
			log.Debug().Err(err).Msg("update check failed")
			return nil
		}
		if result == nil {
			return nil
		}
		return updateAvailableMsg{result: result}
	}
}
