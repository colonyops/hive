package agentws

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/schedule"
)

func validWorkspace() Workspace {
	return Workspace{Version: 1, Name: "X", Command: "claude"}
}

func TestWorkspaceValidate(t *testing.T) {
	t.Parallel()

	t.Run("ValidPasses", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, validWorkspace().Validate())
	})

	t.Run("NameRequired", func(t *testing.T) {
		t.Parallel()
		w := validWorkspace()
		w.Name = ""
		require.Error(t, w.Validate())
	})

	t.Run("CommandRequired", func(t *testing.T) {
		t.Parallel()
		w := validWorkspace()
		w.Command = ""
		require.Error(t, w.Validate())
	})

	t.Run("CommandMustBeAValidTemplate", func(t *testing.T) {
		t.Parallel()
		w := validWorkspace()
		w.Command = "claude {{ .Nonsense }}"
		require.Error(t, w.Validate(), "a template Resolve would refuse must not load")
	})

	t.Run("DuplicateMCPRejected", func(t *testing.T) {
		t.Parallel()
		w := validWorkspace()
		w.MCPs = []string{"playwright", "playwright"}
		require.Error(t, w.Validate())
	})

	t.Run("DuplicateSkillRejected", func(t *testing.T) {
		t.Parallel()
		w := validWorkspace()
		w.Skills = []string{"hive-mcp", "hive-mcp"}
		require.Error(t, w.Validate())
	})

	t.Run("EmptyMCPEntryRejected", func(t *testing.T) {
		t.Parallel()
		w := validWorkspace()
		w.MCPs = []string{""}
		require.Error(t, w.Validate())
	})

	t.Run("EmptySkillEntryRejected", func(t *testing.T) {
		t.Parallel()
		w := validWorkspace()
		w.Skills = []string{""}
		require.Error(t, w.Validate())
	})

	t.Run("SchedulesValidate", func(t *testing.T) {
		t.Parallel()
		w := validWorkspace()
		w.Command += PromptTail
		w.Schedules = []schedule.Spec{
			{ID: "weekly", Cron: "0 9 * * 5", Prompt: "Summarize the week."},
			{ID: "daily", Cron: "@daily", Prompt: "Standup.", OnMissed: schedule.OnMissedSkip},
		}
		require.NoError(t, w.Validate())
	})

	t.Run("BrokenScheduleRejected", func(t *testing.T) {
		t.Parallel()
		w := validWorkspace()
		w.Schedules = []schedule.Spec{{ID: "weekly", Cron: "nope", Prompt: "go"}}
		require.Error(t, w.Validate())
	})

	t.Run("DuplicateScheduleIDRejected", func(t *testing.T) {
		t.Parallel()
		w := validWorkspace()
		w.Schedules = []schedule.Spec{
			{ID: "weekly", Cron: "@daily", Prompt: "go"},
			{ID: "weekly", Cron: "@hourly", Prompt: "go"},
		}
		require.Error(t, w.Validate())
	})
}
