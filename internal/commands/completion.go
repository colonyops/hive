package commands

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/hive"
	"github.com/urfave/cli/v3"
)

// SessionNameCompleter returns a ShellCompleteFunc that suggests active session
// names as positional completions. Set this as the ShellComplete field on any
// cli.Command that accepts session names as arguments.
//
// When the user's last typed argument starts with "-", it falls back to the
// default flag completion behavior.
func SessionNameCompleter(app *hive.App) cli.ShellCompleteFunc {
	return func(ctx context.Context, cmd *cli.Command) {
		// Delegate to default flag completion when typing a flag
		if args := cmd.Args(); args.Present() {
			last := args.Slice()[args.Len()-1]
			if len(last) > 0 && last[0] == '-' {
				cli.DefaultCompleteWithFlags(ctx, cmd)
				return
			}
		}

		sessions, err := app.Sessions.ListSessions(ctx)
		if err != nil {
			return
		}

		w := cmd.Root().Writer
		for _, s := range sessions {
			if s.State != session.StateActive {
				continue
			}
			_, _ = fmt.Fprintln(w, s.Name)
		}
	}
}
