package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

// actionUsage answers "is anything still using this action?" by joining the
// loaded flows with the nonterminal output-command queue. It satisfies
// actions.ActionUsageChecker, which is deliberately narrow so the actions
// catalog depends on neither flow nor the stores.
type actionUsage struct {
	flows    *flow.FlowStore
	commands *stores.OutputCommandStore
}

func newActionUsage(flows *flow.FlowStore, commands *stores.OutputCommandStore) actionUsage {
	return actionUsage{flows: flows, commands: commands}
}

func (c actionUsage) Usage(ctx context.Context, id string) (actions.ActionUsage, error) {
	usage := actions.ActionUsage{}
	for _, f := range c.flows.List() {
		for _, n := range f.Nodes {
			if cfg, ok := n.Config.(*flow.ActionConfig); ok && cfg.Action == id {
				usage.FlowIDs = append(usage.FlowIDs, f.ID)
				break
			}
		}
	}

	count, err := c.commands.CountNonterminalForAction(ctx, id)
	if err != nil {
		return usage, fmt.Errorf("counting nonterminal output commands: %w", err)
	}
	usage.ActiveCommands = count

	sort.Strings(usage.FlowIDs)
	return usage, nil
}
