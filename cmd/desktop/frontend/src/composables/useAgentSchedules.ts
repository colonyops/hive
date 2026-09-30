import { useAgentWorkspaces } from './useAgentWorkspaces'
import type {
  AgentSchedulePreview,
  AgentScheduleRun,
  SchedulePreviewRequest,
} from '../lib/agentWorkspacesClient'

// The schedule calls that are not part of saving a workspace, resolved against
// the same client every other Chats list uses. There is no shared state here:
// a schedule's definition rides the workspace view (AgentWorkspace.schedules),
// so the editor holds the copy being edited and the sidebar reads the saved
// one from the workspace list.

/** How much of a schedule's history the editor's disclosure asks for. */
const RUN_HISTORY_LIMIT = 20

async function resolveClient() {
  const { client, ready } = useAgentWorkspaces()
  await ready()
  if (!client.value) throw new Error('The Chats area is unavailable.')
  return client.value
}

export function useAgentSchedules(): {
  runs: (workspace: string, id: string) => Promise<AgentScheduleRun[]>
  runNow: (workspace: string, id: string) => Promise<AgentScheduleRun>
  preview: (request: SchedulePreviewRequest) => Promise<AgentSchedulePreview>
} {
  return {
    async runs(workspace, id) {
      return await (await resolveClient()).scheduleRuns(workspace, id, RUN_HISTORY_LIMIT)
    },
    async runNow(workspace, id) {
      return await (await resolveClient()).runSchedule(workspace, id)
    },
    async preview(request) {
      return await (await resolveClient()).previewSchedule(request)
    },
  }
}
