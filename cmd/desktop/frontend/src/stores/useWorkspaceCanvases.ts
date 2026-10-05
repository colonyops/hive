import { shallowReadonly } from 'vue'
import { globalCanvasOwner } from '../lib/agentCanvas'
import type { WorkspaceCanvasMeta } from '../lib/agentWorkspacesClient'
import { defineStore } from './defineStore'
import { useAgentWorkspaces } from './useAgentWorkspaces'
import { useResource } from './useResource'

// Every owner's canvas listing in one list, workspaces, repositories and the
// global owner, for the palette's Go-to rows: a canvas pane or page reads its
// own owner through useAgentCanvas instead. Like useAgentSessionsAll, the store does not load
// itself. The palette reloads it on open, after the workspace list it walks.
export const useWorkspaceCanvases = defineStore('workspaceCanvases', () => {
  const { client, ready, workspaces } = useAgentWorkspaces()

  const listing = useResource(
    async () => {
      await ready()
      const agents = client.value
      if (!agents) return []
      const owners = [
        ...workspaces.value.map((workspace) => workspace.dir),
        ...(await agents.canvasRepositories()),
        globalCanvasOwner,
      ]
      const perOwner = await Promise.all(owners.map((owner) => agents.canvases(owner)))
      return perOwner.flat()
    },
    { initial: [] as WorkspaceCanvasMeta[], errorFallback: 'Could not list canvases.' },
  )

  return { canvases: shallowReadonly(listing.data), error: listing.error, reload: listing.reload }
})
