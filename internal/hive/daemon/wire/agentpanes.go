package wire

import (
	"github.com/colonyops/hive/internal/domain/terminal"
	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
	"github.com/colonyops/hive/internal/hive/status"
)

func AgentPanesToProto(sessions map[string][]status.AgentPane) map[string]*daemonv1.AgentPaneList {
	out := make(map[string]*daemonv1.AgentPaneList, len(sessions))
	for id, panes := range sessions {
		list := &daemonv1.AgentPaneList{Panes: make([]*daemonv1.AgentPane, 0, len(panes))}
		for _, p := range panes {
			list.Panes = append(list.Panes, &daemonv1.AgentPane{
				PaneId: p.PaneID, WindowName: p.WindowName, Status: string(p.Status), Tool: p.Tool,
			})
		}
		out[id] = list
	}
	return out
}

func AgentPanesFromProto(sessions map[string]*daemonv1.AgentPaneList) map[string][]status.AgentPane {
	out := make(map[string][]status.AgentPane, len(sessions))
	for id, list := range sessions {
		panes := make([]status.AgentPane, 0, len(list.GetPanes()))
		for _, p := range list.GetPanes() {
			panes = append(panes, status.AgentPane{
				PaneID: p.GetPaneId(), WindowName: p.GetWindowName(), Status: terminal.Status(p.GetStatus()), Tool: p.GetTool(),
			})
		}
		out[id] = panes
	}
	return out
}
