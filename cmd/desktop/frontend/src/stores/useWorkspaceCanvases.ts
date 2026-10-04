import { shallowReadonly } from 'vue'
import type { WorkspaceCanvasMeta } from '../lib/agentWorkspacesClient'
import { defineStore } from './defineStore'
import { useAgentWorkspaces } from './useAgentWorkspaces'
import { useResource } from './useResource'

// Every workspace's canvas listing in one list, for the palette's Go-to rows:
// a canvas pane or page reads its own workspace through useAgentCanvas
// instead. Like useAgentSessionsAll, the store does not load itself. The
// palette reloads it on open, after the workspace list it walks.
export const useWorkspaceCanvases = defineStore('workspaceCanvases', () => {
  const { client, ready, workspaces } = useAgentWorkspaces()

  const listing = useResource(
    async () => {
      await ready()
      const agents = client.value
      if (!agents) return []
      const perWorkspace = await Promise.all(workspaces.value.map((workspace) => agents.canvases(workspace.dir)))
      return perWorkspace.flat()
    },
    { initial: [] as WorkspaceCanvasMeta[], errorFallback: 'Could not list canvases.' },
  )

  return { canvases: shallowReadonly(listing.data), error: listing.error, reload: listing.reload }
})
