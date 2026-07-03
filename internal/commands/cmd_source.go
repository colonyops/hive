package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/sources"
)

// SourceCmd exposes a noninteractive entry point onto the source
// registry: resolve a source, search it, select an item by ID, render
// its session templates, and create a session through the same
// UseBatchSpawn:true path the TUI picker uses. This is the seam integration
// tests drive without a live TUI.
type SourceCmd struct {
	flags *Flags
	app   *hive.App

	scope   string
	pick    string
	query   string
	remote  string
	jsonOut bool
}

// NewSourceCmd creates a new source command.
func NewSourceCmd(flags *Flags, app *hive.App) *SourceCmd {
	return &SourceCmd{flags: flags, app: app}
}

// Register adds the "source" command group.
func (cmd *SourceCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "source",
		Usage: "Source commands (browse external systems, create sessions from items)",
		Commands: []*cli.Command{
			cmd.openCmd(),
		},
	})
	return app
}

func (cmd *SourceCmd) openCmd() *cli.Command {
	return &cli.Command{
		Name:      "open",
		Usage:     "Search a source and create a session from a selected item",
		UsageText: "hive source open <id> --pick <item-id> [--scope <scope>] [--query <query>] [--json]",
		Description: `Resolves <id> from the configured source registry, calls Search (optionally
scoped/filtered), selects the item matching --pick by ID, fetches its detail if the
source supports it, renders the source's configured name/prompt/tags templates
against the selected item, and creates a session via the same batch-spawn path used by
'hive batch'.

This command is a noninteractive equivalent of the TUI source picker, primarily
intended for scripting and integration tests.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "scope",
				Usage:       "source scope (e.g. a GitHub \"owner/name\" repo)",
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
				Usage:       "search query passed to the source",
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

// sourceOpenResult is the JSON shape printed by --json on success.
type sourceOpenResult struct {
	SessionID string   `json:"session_id"`
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Tags      []string `json:"tags,omitempty"`
}

func (cmd *SourceCmd) runOpen(ctx context.Context, c *cli.Command) error {
	sourceID := c.Args().First()
	if sourceID == "" {
		return fmt.Errorf("source id is required: hive source open <id> --pick <item-id>")
	}

	if cmd.app.Sources == nil {
		return fmt.Errorf("no sources are configured")
	}
	conn, tmplCfg, ok := cmd.app.Sources.Get(sourceID)
	if !ok {
		return fmt.Errorf("unknown source %q", sourceID)
	}
	if !conn.Available(ctx) {
		return fmt.Errorf("source %q is not available", sourceID)
	}

	manifest, err := conn.Initialize(ctx)
	if err != nil {
		return fmt.Errorf("source %q: initialize: %w", sourceID, err)
	}

	result, err := conn.Search(ctx, sources.SearchParams{
		Query: cmd.query,
		Scope: cmd.scope,
	})
	if err != nil {
		return fmt.Errorf("source %q: search: %w", sourceID, err)
	}

	item, ok := findItemByID(result.Items, cmd.pick)
	if !ok {
		return fmt.Errorf("source %q: no item with id %q in search results", sourceID, cmd.pick)
	}

	// Detail is optional template data: only fetch it when the source
	// declares the capability (mirrors the TUI picker's gate), and never
	// fail session creation over it being absent.
	detail := item.Detail
	if detail.Kind() == sources.DetailKindNone && manifest.Capabilities.FetchDetail {
		fetched, err := conn.FetchDetail(ctx, sources.FetchDetailParams{
			ID:    item.ID,
			Scope: cmd.scope,
			URI:   item.URI,
		})
		if err != nil {
			return fmt.Errorf("source %q: fetch detail for %q: %w", sourceID, item.ID, err)
		}
		detail = fetched
	}

	rendered, err := sources.RenderSessionTemplates(tmplCfg, item, detail)
	if err != nil {
		return fmt.Errorf("source %q: %w", sourceID, err)
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
		return json.NewEncoder(os.Stdout).Encode(sourceOpenResult{
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
func findItemByID(items []sources.Item, id string) (sources.Item, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return sources.Item{}, false
}
