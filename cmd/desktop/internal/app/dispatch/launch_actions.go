package dispatch

import (
	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

// LaunchNodeActionConfig is a launch node projected onto the launch-session
// executor. It is rebuilt on every resolution, never persisted.
type LaunchNodeActionConfig struct {
	actions.LaunchSessionConfig
	NameTemplate string
}

// launchNodeAction reports false for any other node type, which means the flow
// changed between the graph run and the dispatch.
func launchNodeAction(id string, node flow.Node) (actions.Action, bool) {
	var cfg *LaunchNodeActionConfig
	label := node.Name
	switch c := node.Config.(type) {
	case *flow.LaunchSessionConfig:
		cfg = &LaunchNodeActionConfig{
			PromptTemplate: c.Prompt, RepoTemplate: c.Repo, Agent: c.Agent,
			NameTemplate: c.SessionName,
		}
		if label == "" {
			label = "Launch session " + node.ID
		}
	case *flow.LaunchChatConfig:
		cfg = &LaunchNodeActionConfig{PromptTemplate: c.Prompt, Workspace: c.Workspace}
		if label == "" {
			label = "Launch chat " + node.ID
		}
	default:
		return actions.Action{}, false
	}
	return actions.Action{ID: id, Label: label, Type: ActionTypeLaunchSession, Config: cfg}, true
}
