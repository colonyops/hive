package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/platform/desktopendpoint"
	"github.com/colonyops/hive/pkg/iojson"
)

const (
	desktopDriveMCPPath = "/mcp/drive"
	desktopCallTimeout  = 10 * time.Second

	minSplitShare = 0.2
	maxSplitShare = 0.8
)

// desktopNotRunningError is a type, not errors.New, because the message starts
// with the product name and staticcheck rejects a capitalized error string.
type desktopNotRunningError struct{}

func (desktopNotRunningError) Error() string {
	return "Hive Desktop is not running; start it, or pass --url"
}

var tabPlacements = []string{"behind", "beside", "below"}

func desktopCallFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "session",
			Usage:   "Session the tool acts on (default: $HIVE_AGENT_SESSION, else the current directory)",
			Sources: cli.EnvVars("HIVE_AGENT_SESSION"),
		},
		&cli.StringFlag{
			Name:    "url",
			Usage:   "Base URL of Hive Desktop (default: $HIVE_DESKTOP_URL, else the running desktop)",
			Sources: cli.EnvVars("HIVE_DESKTOP_URL"),
		},
	}
}

func placementFlag(required bool) *cli.StringFlag {
	return &cli.StringFlag{
		Name:     "placement",
		Usage:    "Where the tab goes: " + strings.Join(tabPlacements, ", "),
		Required: required,
	}
}

func validatePlacement(s string) error {
	for _, p := range tabPlacements {
		if s == p {
			return nil
		}
	}
	return fmt.Errorf("invalid: placement %q is not one of %s", s, strings.Join(tabPlacements, ", "))
}

func validateShare(share float64) error {
	if share < minSplitShare || share > maxSplitShare {
		return fmt.Errorf("invalid: share %v is outside %v to %v", share, minSplitShare, maxSplitShare)
	}
	return nil
}

func (cmd *DesktopCmd) layoutCmd() *cli.Command {
	return &cli.Command{
		Name:        "layout",
		Usage:       "Print the tab and panel layout of a session as JSON",
		Description: "Calls the get_layout tool of Hive Desktop.",
		Flags:       desktopCallFlags(),
		Action: func(ctx context.Context, c *cli.Command) error {
			if err := noArgs(c, "layout"); err != nil {
				return err
			}
			return cmd.callTool(ctx, c, "get_layout", nil)
		},
	}
}

func (cmd *DesktopCmd) tabCmd() *cli.Command {
	return &cli.Command{
		Name:  "tab",
		Usage: "Open, close, move and focus tabs",
		Description: `A <tab> is drive, diff, tasks, terminal, or a tmux window of a Code session
by id (@3) or exact name.`,
		Commands: []*cli.Command{
			cmd.tabOpenCmd(),
			cmd.tabCloseCmd(),
			cmd.tabMoveCmd(),
			cmd.tabFocusCmd(),
		},
	}
}

func (cmd *DesktopCmd) tabOpenCmd() *cli.Command {
	return &cli.Command{
		Name:      "open",
		Usage:     "Open a tab, or start a terminal window",
		ArgsUsage: "<drive|diff|tasks|terminal>",
		Description: `--path applies to drive. --window places an existing terminal window; --name
and --command start a new one.`,
		Flags: append(desktopCallFlags(),
			&cli.StringFlag{Name: "path", Usage: "Drive document, or a canvas by name or path (drive only)"},
			placementFlag(false),
			&cli.StringFlag{Name: "anchor", Usage: "Tab the placement is relative to"},
			&cli.StringFlag{Name: "window", Usage: "Existing terminal window to place (terminal only)"},
			&cli.StringFlag{Name: "name", Usage: "Name of a new terminal window (terminal only)"},
			&cli.StringFlag{Name: "command", Usage: "Command a new terminal window runs (terminal only)"},
		),
		Action: func(ctx context.Context, c *cli.Command) error {
			view, err := oneArg(c, "open", "<view>")
			if err != nil {
				return err
			}
			if p := c.String("placement"); p != "" {
				if err := validatePlacement(p); err != nil {
					return err
				}
			}
			args := map[string]any{"view": view}
			for _, name := range []string{"path", "placement", "anchor", "window", "name", "command"} {
				if v := c.String(name); v != "" {
					args[name] = v
				}
			}
			return cmd.callTool(ctx, c, "open_tab", args)
		},
	}
}

func (cmd *DesktopCmd) tabCloseCmd() *cli.Command {
	return &cli.Command{
		Name:      "close",
		Usage:     "Close a tab",
		ArgsUsage: "<tab>",
		Description: `drive, diff and tasks close that view. Anything else is a terminal window of
the session, which is killed.`,
		Flags: desktopCallFlags(),
		Action: func(ctx context.Context, c *cli.Command) error {
			tab, err := oneArg(c, "close", "<tab>")
			if err != nil {
				return err
			}
			args := map[string]any{"view": "terminal", "window": tab}
			switch tab {
			case "drive", "diff", "tasks":
				args = map[string]any{"view": tab}
			}
			return cmd.callTool(ctx, c, "close_tab", args)
		},
	}
}

func (cmd *DesktopCmd) tabMoveCmd() *cli.Command {
	return &cli.Command{
		Name:      "move",
		Usage:     "Move a tab behind, beside or below another tab",
		ArgsUsage: "<tab>",
		Flags: append(desktopCallFlags(),
			placementFlag(true),
			&cli.StringFlag{Name: "anchor", Usage: "Tab the placement is relative to"},
		),
		Action: func(ctx context.Context, c *cli.Command) error {
			tab, err := oneArg(c, "move", "<tab>")
			if err != nil {
				return err
			}
			if err := validatePlacement(c.String("placement")); err != nil {
				return err
			}
			args := map[string]any{"tab": tab, "placement": c.String("placement")}
			if anchor := c.String("anchor"); anchor != "" {
				args["anchor"] = anchor
			}
			return cmd.callTool(ctx, c, "move_tab", args)
		},
	}
}

func (cmd *DesktopCmd) tabFocusCmd() *cli.Command {
	return &cli.Command{
		Name:      "focus",
		Usage:     "Bring a tab to the front of its panel",
		ArgsUsage: "<tab>",
		Flags:     desktopCallFlags(),
		Action: func(ctx context.Context, c *cli.Command) error {
			tab, err := oneArg(c, "focus", "<tab>")
			if err != nil {
				return err
			}
			return cmd.callTool(ctx, c, "focus_tab", map[string]any{"tab": tab})
		},
	}
}

func (cmd *DesktopCmd) panelCmd() *cli.Command {
	return &cli.Command{
		Name:  "panel",
		Usage: "Close panels",
		Commands: []*cli.Command{
			{
				Name:        "close",
				Usage:       "Close the panel that holds a tab",
				ArgsUsage:   "<tab>",
				Description: "Closes every tab in the panel.",
				Flags:       desktopCallFlags(),
				Action: func(ctx context.Context, c *cli.Command) error {
					tab, err := oneArg(c, "close", "<tab>")
					if err != nil {
						return err
					}
					return cmd.callTool(ctx, c, "close_panel", map[string]any{"tab": tab})
				},
			},
		},
	}
}

func (cmd *DesktopCmd) splitCmd() *cli.Command {
	return &cli.Command{
		Name:  "split",
		Usage: "Resize and turn splits",
		Commands: []*cli.Command{
			{
				Name:      "resize",
				Usage:     "Give the side that holds a tab a share of its nearest split",
				ArgsUsage: "<tab>",
				Flags: append(desktopCallFlags(),
					&cli.FloatFlag{Name: "share", Usage: "Share of the split, 0.2 to 0.8", Required: true},
				),
				Action: func(ctx context.Context, c *cli.Command) error {
					tab, err := oneArg(c, "resize", "<tab>")
					if err != nil {
						return err
					}
					share := c.Float("share")
					if err := validateShare(share); err != nil {
						return err
					}
					return cmd.callTool(ctx, c, "resize_split", map[string]any{"tab": tab, "share": share})
				},
			},
			{
				Name:      "turn",
				Usage:     "Turn the nearest split of a tab between side by side and stacked",
				ArgsUsage: "<tab>",
				Flags:     desktopCallFlags(),
				Action: func(ctx context.Context, c *cli.Command) error {
					tab, err := oneArg(c, "turn", "<tab>")
					if err != nil {
						return err
					}
					return cmd.callTool(ctx, c, "turn_split", map[string]any{"tab": tab})
				},
			},
		},
	}
}

func noArgs(c *cli.Command, name string) error {
	if c.Args().Len() != 0 {
		return fmt.Errorf("%s takes no arguments", name)
	}
	return nil
}

func oneArg(c *cli.Command, name, usage string) (string, error) {
	if c.Args().Len() != 1 {
		return "", fmt.Errorf("%s takes one %s", name, usage)
	}
	return c.Args().First(), nil
}

// callTool runs one hive-drive tool and prints its structured result as a
// JSON line. A refused call comes back as the tool's own text, unwrapped, so
// the message reads the same here as it does to an agent over MCP.
func (cmd *DesktopCmd) callTool(ctx context.Context, c *cli.Command, tool string, args map[string]any) error {
	baseURL, err := cmd.desktopURL(c)
	if err != nil {
		return err
	}
	session, err := desktopSession(c)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, desktopCallTimeout)
	defer cancel()

	// The standalone SSE stream carries server-initiated messages, which a
	// one-shot call never reads. Skipping it saves a request, and a negative
	// MaxRetries stops a refused connection from retrying inside the timeout.
	transport := &mcp.StreamableClientTransport{
		Endpoint:             strings.TrimRight(baseURL, "/") + desktopDriveMCPPath,
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "hive", Version: cmd.app.Build.Version}, nil)
	cs, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return desktopConnectError(baseURL, err)
	}
	defer func() { _ = cs.Close() }()

	if args == nil {
		args = map[string]any{}
	}
	args["session"] = session

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return desktopConnectError(baseURL, fmt.Errorf("call %s: %w", tool, err))
	}
	if res.IsError {
		return errors.New(toolResultText(res, tool))
	}
	return iojson.WriteLine(c.Root().Writer, toolResultValue(res))
}

func (cmd *DesktopCmd) desktopURL(c *cli.Command) (string, error) {
	if u := c.String("url"); u != "" {
		return u, nil
	}
	e, err := desktopendpoint.Read(cmd.app.Config.DataDir)
	if errors.Is(err, desktopendpoint.ErrNotRunning) {
		return "", desktopNotRunningError{}
	}
	if err != nil {
		return "", err
	}
	return e.URL, nil
}

func desktopSession(c *cli.Command) (string, error) {
	if s := c.String("session"); s != "" {
		return s, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("find current directory: %w", err)
	}
	return filepath.Abs(wd)
}

// desktopConnectError names the usual cause of a failed connection. A stale
// endpoint file left by a crashed desktop lands here too, since Read does not
// probe the PID.
func desktopConnectError(baseURL string, err error) error {
	if errors.Is(err, syscall.ECONNREFUSED) || strings.Contains(err.Error(), "connection refused") {
		return desktopNotRunningError{}
	}
	return fmt.Errorf("reach Hive Desktop at %s: %w", baseURL, err)
}

func toolResultText(res *mcp.CallToolResult, tool string) string {
	var parts []string
	for _, content := range res.Content {
		if text, ok := content.(*mcp.TextContent); ok && text.Text != "" {
			parts = append(parts, text.Text)
		}
	}
	if len(parts) == 0 {
		return tool + " failed"
	}
	return strings.Join(parts, "\n")
}

// toolResultValue prefers the structured result and falls back to the text
// content, which holds the same JSON for tools that return a typed result.
func toolResultValue(res *mcp.CallToolResult) any {
	if res.StructuredContent != nil {
		return res.StructuredContent
	}
	for _, content := range res.Content {
		text, ok := content.(*mcp.TextContent)
		if !ok {
			continue
		}
		var v any
		if json.Unmarshal([]byte(text.Text), &v) == nil && v != nil {
			return v
		}
	}
	return map[string]any{}
}
