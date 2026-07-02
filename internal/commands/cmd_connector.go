package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/connectors"
	"github.com/colonyops/hive/internal/hive"
)

// ConnectorCmd exposes a noninteractive entry point onto the connector
// registry: resolve a connector, search it, select an item by ID, render
// its session templates, and create a session through the same
// UseBatchSpawn:true path the TUI picker uses. This is the seam integration
// tests drive without a live TUI.
type ConnectorCmd struct {
	flags *Flags
	app   *hive.App

	scope   string
	pick    string
	query   string
	remote  string
	jsonOut bool
}

// NewConnectorCmd creates a new connector command.
func NewConnectorCmd(flags *Flags, app *hive.App) *ConnectorCmd {
	return &ConnectorCmd{flags: flags, app: app}
}

// Register adds the "connector" command group.
func (cmd *ConnectorCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "connector",
		Usage: "Connector commands (browse external systems, create sessions from items)",
		Commands: []*cli.Command{
			cmd.openCmd(),
		},
	})
	return app
}

func (cmd *ConnectorCmd) openCmd() *cli.Command {
	return &cli.Command{
		Name:      "open",
		Usage:     "Search a connector and create a session from a selected item",
		UsageText: "hive connector open <id> --pick <item-id> [--scope <scope>] [--query <query>] [--json]",
		Description: `Resolves <id> from the configured connector registry, calls Search (optionally
scoped/filtered), selects the item matching --pick by ID, fetches its detail if the
connector supports it, renders the connector's configured name/prompt/tags templates
against the selected item, and creates a session via the same batch-spawn path used by
'hive batch'.

This command is a noninteractive equivalent of the TUI connector picker, primarily
intended for scripting and integration tests.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "scope",
				Usage:       "connector scope (e.g. a GitHub \"owner/name\" repo)",
				Destination: &cmd.scope,
			},
			&cli.StringFlag{
				Name:        "pick",
				Usage:       "id of the item to select from the search results",
				Destination: &cmd.pick,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "query",
				Usage:       "search query passed to the connector",
				Destination: &cmd.query,
			},
			&cli.StringFlag{
				Name:        "remote",
				Usage:       "git remote URL to clone (auto-detected from cwd if omitted)",
				Destination: &cmd.remote,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "output the created session as JSON",
				Destination: &cmd.jsonOut,
			},
		},
		Action: cmd.runOpen,
	}
}

// connectorOpenResult is the JSON shape printed by --json on success.
type connectorOpenResult struct {
	SessionID string   `json:"session_id"`
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Tags      []string `json:"tags,omitempty"`
}

func (cmd *ConnectorCmd) runOpen(ctx context.Context, c *cli.Command) error {
	connectorID := c.Args().First()
	if connectorID == "" {
		return fmt.Errorf("connector id is required: hive connector open <id> --pick <item-id>")
	}

	if cmd.app.Connectors == nil {
		return fmt.Errorf("no connectors are configured")
	}
	conn, tmplCfg, ok := cmd.app.Connectors.Get(connectorID)
	if !ok {
		return fmt.Errorf("unknown connector %q", connectorID)
	}
	if !conn.Available(ctx) {
		return fmt.Errorf("connector %q is not available", connectorID)
	}

	result, err := conn.Search(ctx, connectors.SearchParams{
		Query: cmd.query,
		Scope: cmd.scope,
	})
	if err != nil {
		return fmt.Errorf("connector %q: search: %w", connectorID, err)
	}

	item, ok := findItemByID(result.Items, cmd.pick)
	if !ok {
		return fmt.Errorf("connector %q: no item with id %q in search results", connectorID, cmd.pick)
	}

	detail := item.Detail
	if detail.Kind() == connectors.DetailKindNone {
		fetched, err := conn.FetchDetail(ctx, connectors.FetchDetailParams{
			ID:    item.ID,
			Scope: cmd.scope,
			URI:   item.URI,
		})
		if err != nil {
			return fmt.Errorf("connector %q: fetch detail for %q: %w", connectorID, item.ID, err)
		}
		detail = fetched
	}

	rendered, err := connectors.RenderSessionTemplates(tmplCfg, item, detail)
	if err != nil {
		return fmt.Errorf("connector %q: %w", connectorID, err)
	}

	created, err := cmd.app.Sessions.CreateSession(ctx, hive.CreateOptions{
		Name:          rendered.Name,
		Prompt:        rendered.Prompt,
		Tags:          rendered.Tags,
		Remote:        cmd.remote,
		UseBatchSpawn: true,
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	if cmd.jsonOut {
		return json.NewEncoder(os.Stdout).Encode(connectorOpenResult{
			SessionID: created.ID,
			Name:      created.Name,
			Path:      created.Path,
			Tags:      rendered.Tags,
		})
	}

	if _, err := fmt.Fprintf(os.Stdout, "created session %s (%s)\n", created.Name, created.ID); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

// findItemByID returns the item in items whose ID matches id.
func findItemByID(items []connectors.Item, id string) (connectors.Item, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return connectors.Item{}, false
}
