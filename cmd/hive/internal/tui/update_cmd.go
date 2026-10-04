package tui

import (
	"context"

	"github.com/rs/zerolog"

	tea "charm.land/bubbletea/v2"
	"github.com/colonyops/hive/cmd/hive/internal/updatecheck"
)

type updateAvailableMsg struct {
	result *updatecheck.Result
}

func checkForUpdate(logger zerolog.Logger, checker *updatecheck.Checker, currentVersion string) tea.Cmd {
	return func() tea.Msg {
		if checker == nil {
			return nil
		}
		result, err := checker.Check(context.Background(), currentVersion)
		if err != nil {
			logger.Debug().Err(err).Msg("update check failed")
			return nil
		}
		if result == nil {
			return nil
		}
		return updateAvailableMsg{result: result}
	}
}
