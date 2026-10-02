package dispatch

import (
	"fmt"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

// ActionTypeNotify is the action type the notify executor is registered
// under. Unlike "shell" or "publish-message" it is never authored in
// actions.yml: a notify node carries its config inline, and
// FlowActions synthesizes the Action the dispatcher needs from the
// live flow set.
const ActionTypeNotify = "notify"

// NotifyActionConfig is the executor-facing projection of a notify node's
// flow.NotifyConfig. It exists because the output worker speaks in
// actions.Action values (whose Config is an actions.ActionConfig), while the
// authoritative definition stays the flow node — this type is rebuilt from
// that node on every resolution, never persisted.
type NotifyActionConfig struct {
	Title    string
	Body     string
	Severity string
	Sound    bool
	// Cooldown is the node's resolved per-item delivery floor:
	// flow.NotifyCooldownDefault when the node declares none, 0 when it
	// explicitly disabled the cooldown.
	Cooldown time.Duration
}

// Validate satisfies actions.ActionConfig. The flow's own validator is
// authoritative (SaveFlow rejects a notify node without a title before it can
// ever reach a queue), so this only re-states the one invariant the executor
// depends on.
func (c *NotifyActionConfig) Validate() error {
	if strings.TrimSpace(c.Title) == "" {
		return fmt.Errorf("notify: title is required")
	}
	return nil
}

// notifyRaiser is satisfied by a node config that can raise a notify output:
// a notify node, whose whole purpose it is. It is declared here, the
// consumer, and satisfied structurally by flow.NotifyConfig via a method
// declared alongside it — a declared capability in place of a switch over
// concrete config types. TestNotifyRaiserCoversExactlyNotify guards that the
// set of node types satisfying it stays exactly {notify}, so a future
// terminal node type cannot silently fall through the assertion below.
type notifyRaiser interface {
	// NotifyDeclaration returns the notify content to raise, or ok=false if
	// this config does not raise a notify output.
	NotifyDeclaration() (cfg *flow.NotifyConfig, ok bool)
}

// FlowLister is the subset of *flow.FlowStore this package needs: the
// current set of loaded flows. It is declared per consuming package rather
// than shared, so a package's dependency on the flow store is exactly the
// method it calls.
type FlowLister interface {
	List() []flow.Flow
}

// FlowActions resolves the synthetic ids flow terminals enqueue under (notify
// and launch nodes) from live flows, and delegates other IDs to the authored
// action catalog. Resolution happens per lookup so flow edits apply without a
// restart.
type FlowActions struct {
	flows   FlowLister
	actions ActionLister
}

// NewFlowActions wraps an authored action store with flow-node resolution.
func NewFlowActions(flows FlowLister, catalog ActionLister) *FlowActions {
	return &FlowActions{flows: flows, actions: catalog}
}

// Get resolves id to an executable action. A synthetic id that no longer
// names a matching node in any flow is reported as unknown, exactly like a
// deleted actions.yml entry: its queued command fails rather than silently
// doing nothing.
func (l *FlowActions) Get(id string) (actions.Action, bool) {
	if target, ok := models.NotifyActionTarget(id); ok {
		node, found := l.node(target)
		if !found {
			return actions.Action{}, false
		}
		return notifyNodeAction(id, node)
	}
	if target, ok := models.LaunchActionTarget(id); ok {
		node, found := l.node(target)
		if !found {
			return actions.Action{}, false
		}
		return launchNodeAction(id, node)
	}
	if l.actions == nil {
		return actions.Action{}, false
	}
	return l.actions.Get(id)
}

// node finds the node a flow-qualified "<flowId>/<nodeId>" target names.
func (l *FlowActions) node(target string) (flow.Node, bool) {
	if l.flows == nil {
		return flow.Node{}, false
	}
	flowID, nodeID, found := strings.Cut(target, "/")
	if !found {
		return flow.Node{}, false
	}
	for _, f := range l.flows.List() {
		if f.ID != flowID {
			continue
		}
		for _, node := range f.Nodes {
			if node.ID == nodeID {
				return node, true
			}
		}
	}
	return flow.Node{}, false
}

func notifyNodeAction(id string, node flow.Node) (actions.Action, bool) {
	cfg, ok := notifyNodeConfig(node)
	if !ok {
		return actions.Action{}, false
	}
	return actions.Action{
		ID:    id,
		Label: notifyLabel(node),
		Type:  ActionTypeNotify,
		Config: &NotifyActionConfig{
			Title:    cfg.Title,
			Body:     cfg.Body,
			Severity: cfg.SeverityOrDefault(),
			Sound:    cfg.SoundOrDefault(),
			Cooldown: cfg.CooldownOrDefault(),
		},
	}, true
}

// notifyNodeConfig reads the notify content a node delivers through from
// whatever the node's own config declares via notifyRaiser. A node whose
// config raises no notify output reports ok=false — reaching here for one
// means the flow was edited between the graph run and the delivery, reported
// as unresolved just like a deleted node.
func notifyNodeConfig(node flow.Node) (cfg *flow.NotifyConfig, ok bool) {
	raiser, ok := node.Config.(notifyRaiser)
	if !ok {
		return nil, false
	}
	return raiser.NotifyDeclaration()
}

// notifyLabel is the human name a notify delivery reports in the Activity
// view and the jobs list: the node's author-given name, else its id.
func notifyLabel(node flow.Node) string {
	if node.Name != "" {
		return node.Name
	}
	return "Notify " + node.ID
}
