package commands

import (
	"testing"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
)

// commandTree builds the hive command tree the way cmd/hive/cli does, for
// tests that check a rule across every command.
func commandTree(t *testing.T) *cli.Command {
	t.Helper()
	return registerCommands(&cli.Command{Name: "hive"}, testApp(t))
}

func registerCommands(root *cli.Command, a *app.App) *cli.Command {
	flags := &Flags{}
	root.Flags = append(root.Flags, NewTuiCmd(flags, a).Flags()...)
	for _, register := range []func(*cli.Command) *cli.Command{
		NewNewCmd(flags, a).Register,
		NewPruneCmd(flags, a).Register,
		NewDoctorCmd(flags, a).Register,
		NewBatchCmd(flags, a).Register,
		NewDriveCmd(flags, a).Register,
		NewMsgCmd(flags, a).Register,
		NewDocCmd(flags, a).Register,
		NewSessionCmd(flags, a).Register,
		NewReviewCmd(flags, a).Register,
		NewTodoCmd(flags, a).Register,
		NewConfigCmd(flags, a).Register,
		NewDetectCmd(flags, a).Register,
		NewHoneycombCmd(flags, a).Register,
		NewWorkspaceCmd(flags, a).Register,
		NewInitCmd(flags, a).Register,
		NewDesktopCmd(flags, a).Register,
		NewMCPCmd(flags, a).Register,
		NewExperimentalCmd(flags, a).Register,
		NewDaemonCmd(flags, a).Register,
		NewServersCmd(flags, a).Register,
	} {
		root = register(root)
	}
	return root
}
