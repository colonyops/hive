package dispatch

import (
	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

// publishNodeAction projects a publish-message node onto the publish-message
// executor, so the node publishes exactly as the catalog action would.
func publishNodeAction(id string, node flow.Node) (actions.Action, bool) {
	cfg, ok := node.Config.(*flow.PublishMessageConfig)
	if !ok {
		return actions.Action{}, false
	}
	label := node.Name
	if label == "" {
		label = "Publish message " + node.ID
	}
	return actions.Action{
		ID: id, Label: label, Type: ActionTypePublishMessage,
		Config: &actions.PublishMessageConfig{MessageTemplate: cfg.Message, Topic: cfg.Topic},
	}, true
}

func taskNodeAction(id string, node flow.Node) (actions.Action, bool) {
	cfg, ok := node.Config.(*flow.CreateTaskConfig)
	if !ok {
		return actions.Action{}, false
	}
	label := node.Name
	if label == "" {
		label = "Create task " + node.ID
	}
	return actions.Action{
		ID: id, Label: label, Type: ActionTypeCreateTask,
		Config: &CreateTaskActionConfig{
			TitleTemplate: cfg.Title, DescriptionTemplate: cfg.Description, RepoTemplate: cfg.Repo, Parent: cfg.Parent,
			Tags: cfg.Tags,
		},
	}, true
}
