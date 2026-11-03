package mcpsrv

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/internal/ops/flows"
	"github.com/colonyops/hive/internal/ops/workspaces"

	"github.com/colonyops/hive/internal/ops/actions"
	"github.com/colonyops/hive/internal/ops/diagnostics"
	"github.com/colonyops/hive/internal/ops/inbox"
	"github.com/colonyops/hive/internal/ops/mcpops"
	"github.com/colonyops/hive/internal/ops/sessions"
)

// register mounts every tool this adapter serves. The tool table is the
// catalog in internal/ops: each Spec carries the metadata, and each handler
// here is a thin call into *app.App (ADR mcp-replaces-the-agent-facing-http-api).
func (ctrl *Controller) register(srv *mcp.Server) {
	mcpops.Mount(srv, diagnostics.StatusGet, ctrl.GetStatus, ctrl.toolError)
	mcpops.Mount(srv, diagnostics.DiagnosticsGet, ctrl.ReadDiagnostics, ctrl.toolError)

	mcpops.Mount(srv, inbox.FeedList, ctrl.ListFeeds, ctrl.toolError)
	mcpops.Mount(srv, inbox.InboxList, ctrl.ListInbox, ctrl.toolError)
	mcpops.Mount(srv, inbox.InboxEventList, ctrl.ListInboxItemEvents, ctrl.toolError)
	mcpops.Mount(srv, inbox.InboxSessionList, ctrl.ListItemSessions, ctrl.toolError)
	mcpops.Mount(srv, inbox.SourceRefresh, ctrl.RefreshSources, ctrl.toolError)

	mcpops.Mount(srv, sessions.SessionList, ctrl.ListSessions, ctrl.toolError)
	mcpops.Mount(srv, sessions.SessionTagsUpdate, ctrl.EditSessionTags, ctrl.toolError)

	mcpops.Mount(srv, actions.ActionList, ctrl.ListActions, ctrl.toolError)
	mcpops.Mount(srv, actions.ActionRunList, ctrl.ListActionRuns, ctrl.toolError)
	mcpops.Mount(srv, actions.ActionRunGet, ctrl.GetActionRun, ctrl.toolError)
	mcpops.Mount(srv, actions.AppletList, ctrl.ListApplets, ctrl.toolError)

	mcpops.Mount(srv, flows.ProfileList, ctrl.ListProfiles, ctrl.toolError)
	mcpops.Mount(srv, flows.ProfileCreate, ctrl.CreateProfile, ctrl.toolError)
	mcpops.Mount(srv, flows.ProfileDelete, ctrl.DeleteProfile, ctrl.toolError)
	mcpops.MountResult(srv, flows.ProfileImageGet, ctrl.GetProfileImage, ctrl.toolError)
	mcpops.Mount(srv, flows.ProfileImagePut, ctrl.SetProfileImage, ctrl.toolError)
	mcpops.Mount(srv, flows.ProfileImageDelete, ctrl.ClearProfileImage, ctrl.toolError)
	mcpops.Mount(srv, flows.FlowGet, ctrl.GetFlow, ctrl.toolError)
	mcpops.Mount(srv, flows.FlowPreview, ctrl.ExecuteFlow, ctrl.toolError)
	mcpops.MountResult(srv, flows.NodeImageGet, ctrl.GetNodeImage, ctrl.toolError)
	mcpops.Mount(srv, flows.NodeImagePut, ctrl.SetNodeImage, ctrl.toolError)
	mcpops.Mount(srv, flows.NodeImageDelete, ctrl.ClearNodeImage, ctrl.toolError)

	mcpops.Mount(srv, workspaces.WorkspaceList, ctrl.ListWorkspaces, ctrl.toolError)
	mcpops.Mount(srv, workspaces.ScheduleList, ctrl.ListSchedules, ctrl.toolError)
	mcpops.Mount(srv, workspaces.SchedulePut, ctrl.PutSchedule, ctrl.toolError)
	mcpops.Mount(srv, workspaces.ScheduleDelete, ctrl.RemoveSchedule, ctrl.toolError)
	mcpops.Mount(srv, workspaces.SchedulePreview, ctrl.PreviewSchedule, ctrl.toolError)
	mcpops.Mount(srv, workspaces.ScheduleRunList, ctrl.ScheduleRuns, ctrl.toolError)
}
