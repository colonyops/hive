package commands

import (
	"context"
	"fmt"
	"io"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/hive"
	"github.com/urfave/cli/v3"
)

// SessionNameCompleter returns a ShellCompleteFunc that suggests active session
// names as positional completions. Set this as the ShellComplete field on any
// cli.Command that accepts session names as arguments.
//
// When the app is not initialized (e.g. during lightweight completion mode) or
// when the user's last typed argument starts with "-", it falls back to the
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

		// Fall back to default completion if the app wasn't initialized
		// (e.g. the Before hook was skipped during completion mode).
		if app.Sessions == nil {
			cli.DefaultCompleteWithFlags(ctx, cmd)
			return
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

// ConfigureCompletionCommand customises the auto-generated completion command.
// It replaces the zsh action with a custom script using compadd (the upstream
// urfave/cli _describe-based script inserts "name:description" as one word on
// some zsh configurations). Bash, fish, and pwsh use the built-in scripts.
func ConfigureCompletionCommand(cc *cli.Command) {
	cc.Hidden = false
	cc.Usage = "Generate shell completion scripts"
	cc.Description = `Generate shell completion scripts for bash, zsh, fish, or powershell.

To load completions:

Bash:
  source <(hive completion bash)

  # To load completions for each session, add to ~/.bashrc:
  source <(hive completion bash)

Zsh:
  source <(hive completion zsh)

  # Or generate a file:
  hive completion zsh > "${fpath[1]}/_hive"

Fish:
  hive completion fish > ~/.config/fish/completions/hive.fish

PowerShell:
  hive completion pwsh > hive.ps1 && . ./hive.ps1`

	// Wrap the original action so we can intercept "zsh".
	origAction := cc.Action
	cc.Action = func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().First() == "zsh" {
			return writeZshCompletion(cmd.Root().Writer, cmd.Root().Name)
		}
		return origAction(ctx, cmd)
	}
}

// zshCompletionTmpl is a custom zsh completion script that uses compadd
// instead of _describe. The upstream urfave/cli script uses _describe which
// inserts the full "name:description" string on some zsh configurations.
const zshCompletionTmpl = `#compdef %[1]s
compdef _%[1]s %[1]s

_%[1]s() {
	local -a candidates displays
	local line

	if [[ "${words[-1]}" == "-"* ]]; then
		while IFS= read -r line; do
			candidates+=("${line%%%%:*}")
			displays+=("$line")
		done < <(${words[@]:0:#words[@]-1} ${words[-1]} --generate-shell-completion 2>/dev/null)
	else
		while IFS= read -r line; do
			candidates+=("${line%%%%:*}")
			displays+=("$line")
		done < <(${words[@]:0:#words[@]-1} --generate-shell-completion 2>/dev/null)
	fi

	if (( ${#candidates} )); then
		compadd -l -d displays -a candidates
	else
		_files
	fi
}

if [ "$funcstack[1]" = "_%[1]s" ]; then
	_%[1]s
fi
`

func writeZshCompletion(w io.Writer, appName string) error {
	_, err := fmt.Fprintf(w, zshCompletionTmpl, appName)
	return err
}
