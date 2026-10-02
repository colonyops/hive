package dispatch

import (
	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

// LaunchNodeActionConfig is the executor-facing projection of a launch-session
// or launch-chat flow node: the launch-session action fields the node maps
// onto, plus the session name template only a node can declare. Like
// NotifyActionConfig it is rebuilt from the node on every resolution, never
// persisted.
type LaunchNodeActionConfig struct {
	actions.LaunchSessionConfig
	// NameTemplate renders the session's name. Empty derives one from the
	// action id and the item key, as a catalog action does.
	NameTemplate string
}

// launchNodeAction projects a launch node onto the launch-session executor,
// so a node launches exactly as a catalog action with the same fields would.
// A node of any other type reports ok=false: the flow changed between the
// graph run and the dispatch.
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
