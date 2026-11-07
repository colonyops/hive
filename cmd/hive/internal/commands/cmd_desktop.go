package commands

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/cmd/hive/internal/opscli"
	"github.com/colonyops/hive/internal/domain/deeplink"
	"github.com/colonyops/hive/internal/ops/catalog"
	"github.com/colonyops/hive/pkg/logutils"
)

// DesktopCmd implements the hive desktop command group.
type DesktopCmd struct {
	flags *Flags
	app   *app.App
}

// NewDesktopCmd creates a new desktop command group.
func NewDesktopCmd(flags *Flags, app *app.App) *DesktopCmd {
	return &DesktopCmd{flags: flags, app: app}
}

// Register adds the desktop command group to the application.
func (cmd *DesktopCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:    "desktop",
		Aliases: []string{"dt"},
		Usage:   "Work with Hive Desktop (experimental)",
		Description: `Experimental: the commands and the link format can change.

Hive Desktop registers hive:// links with the operating system. A link raises
the window and opens the place it names:

  hive://code/<slug>[?server=<name>][&window=<id>]
  hive://chats/<workspace>[/<chat-id>]
  hive://inbox/<profile>[/<item-id>][?feed=<id>]
  hive://drive/<space>[/<path>]

<slug> is a tmux session name. server= names a server under servers: in
config.yaml; without it a code link names a session on this machine. A drive
<space> is shared, workspaces/<name>, or <owner>/<repo>, and <path> is a document or folder inside it, such as
hive://drive/colonyops/hive/plans/x.md; with no path the link opens the
space. Escape each part, so "release plan" is release%20plan.

The running desktop also serves its operations to the CLI, one command per
MCP tool: "hive dt tab open drive" calls tab_open. Each takes its fields as
flags, or the whole input as JSON with --input. They print the result as one
JSON line and exit with a code for the kind of a refused call: invalid 2,
not_found 3, conflict 4, unavailable 5, unauthenticated 6, any other 1.

They act for --caller (default: $HIVE_AGENT_SESSION, else the current
directory) and find the desktop through its endpoint file, or --desktop-url.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    callerFlag,
				Usage:   "Who the call acts for (default: $HIVE_AGENT_SESSION, else the current directory)",
				Sources: cli.EnvVars("HIVE_AGENT_SESSION"),
			},
			&cli.StringFlag{
				Name:    desktopURLFlag,
				Usage:   "Base URL of Hive Desktop (default: $HIVE_DESKTOP_URL, else the running desktop)",
				Sources: cli.EnvVars("HIVE_DESKTOP_URL"),
			},
		},
		Commands: append([]*cli.Command{cmd.openCmd()}, cmd.opCommands()...),
	})
	return app
}

// opCommands builds a command for every desktop operation in the catalog.
// A spec the CLI cannot spell is a programming error that the parity test
// catches.
func (cmd *DesktopCmd) opCommands() []*cli.Command {
	commands, err := opscli.DesktopCommands(catalog.All(), cmd.call, desktopCaller, callerFlag, desktopURLFlag)
	if err != nil {
		panic(err)
	}
	for _, group := range commands {
		for _, leaf := range group.Commands {
			leaf.Metadata = alwaysJSON()
		}
	}
	return commands
}

func (cmd *DesktopCmd) openCmd() *cli.Command {
	return &cli.Command{
		Name:      "open",
		Usage:     "Open a hive:// link in Hive Desktop",
		ArgsUsage: "<hive://...>",
		Description: `Checks the link and opens it through the operating system, which starts Hive
Desktop if it is not running. See 'hive desktop --help' for the link format.`,
		Action: cmd.runOpen,
	}
}

func (cmd *DesktopCmd) runOpen(ctx context.Context, c *cli.Command) error {
	if c.Args().Len() != 1 {
		return invalidf("open takes one hive:// link")
	}
	link := c.Args().First()
	if _, err := deeplink.Parse(link); err != nil {
		return err
	}

	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	log := logutils.Component(cmd.app.Logger, "cli.desktop").With().Str("url", link).Str("opener", opener).Logger()
	log.Info().Msg("opening hive:// link")

	out, err := exec.CommandContext(ctx, opener, link).CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		log.Error().Err(err).Str("output", msg).Msg("opener failed")
		if msg != "" {
			return fmt.Errorf("%s %s: %w: %s", opener, link, err, msg)
		}
		return fmt.Errorf("%s %s: %w", opener, link, err)
	}
	// open and xdg-open exit 0 once the OS accepts the URL, which says nothing
	// about whether Hive Desktop received it; the desktop log records that.
	log.Info().Str("output", msg).Msg("opener accepted hive:// link")
	return nil
}
