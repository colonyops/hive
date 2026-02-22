package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/colonyops/hive/internal/hive/updatecheck"
	"github.com/rs/zerolog/log"
)

type updateAvailableMsg struct {
	result *updatecheck.Result
}

func checkForUpdate(checker *updatecheck.Checker, currentVersion string) tea.Cmd {
	return func() tea.Msg {
		if checker == nil {
			return nil
		}
		result, err := checker.Check(context.Background(), currentVersion)
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
