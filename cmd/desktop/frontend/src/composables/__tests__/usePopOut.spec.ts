import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useDrive } from '../../stores/useDrive'
import { doc, folder, root } from '../../test-utils/drive'
import { usePopOut } from '../usePopOut'
import { useToasts } from '../useToasts'

const api = vi.hoisted(() => ({ Open: vi.fn(), Tree: vi.fn(), Spaces: vi.fn() }))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/popoutservice', () => ({
  Open: api.Open,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/driveservice', () => ({
  Tree: api.Tree,
  Spaces: api.Spaces,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/windowservice', () => ({
  Focused: vi.fn().mockResolvedValue(true),
}))

const wailsEvents = vi.hoisted(() => ({
  handlers: [] as Array<[string, (event: { data: unknown }) => void]>,
}))
vi.mock('../useWailsEvent', () => ({
  useWailsEvent: (name: string, handler: (event: { data: unknown }) => void) => {
    wailsEvents.handlers.push([name, handler])
  },
}))

function agentWrote(session: string, space: string, path: string): void {
  for (const [name, handler] of wailsEvents.handlers) {
    if (name === 'drive:updated') handler({ data: { space, paths: [path], session, hiveSession: '' } })
  }
}

const space = 'workspaces/web-app'

describe('usePopOut', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    wailsEvents.handlers = []
    api.Open.mockResolvedValue({ url: '/?popout=drive', opened: true })
    api.Spaces.mockResolvedValue([])
  })

  it("pins the session's last write, which a new window could not know", async () => {
    const popOut = usePopOut()
    agentWrote('7', space, 'plans/design.md')

    popOut({ space, path: null, session: '7' })
    await flushPromises()

    expect(api.Open).toHaveBeenCalledWith({
      kind: 'drive',
      params: { space, doc: 'plans/design.md', session: '7' },
    })
  })

  it('keeps a path the caller already pinned', async () => {
    const popOut = usePopOut()
    agentWrote('7', space, 'plans/design.md')

    popOut({ space, path: 'notes.md', session: '7' })
    await flushPromises()

    expect(api.Open).toHaveBeenCalledWith({ kind: 'drive', params: { space, doc: 'notes.md', session: '7' } })
  })

  it('leaves the default to the new window when the session wrote nothing', async () => {
    const popOut = usePopOut()

    popOut({ space, path: null, session: '7' })
    await flushPromises()

    expect(api.Open).toHaveBeenCalledWith({ kind: 'drive', params: { space, session: '7' } })
  })

  it('does not pin a last write that has left the tree', async () => {
    api.Tree.mockResolvedValue(root([folder('plans', [doc('plans/other.md')])]))
    await useDrive().ensureTree(space)
    const popOut = usePopOut()
    agentWrote('7', space, 'plans/deleted.md')

    popOut({ space, path: null, session: '7' })
    await flushPromises()

    expect(api.Open).toHaveBeenCalledWith({ kind: 'drive', params: { space, session: '7' } })
  })

  it('opens the URL itself in a build with no native windows', async () => {
    api.Open.mockResolvedValue({ url: '/?popout=drive&space=x', opened: false })
    const open = vi.spyOn(window, 'open').mockReturnValue(null)

    usePopOut()({ space: 'x', path: null, session: null })
    await flushPromises()

    expect(open).toHaveBeenCalledWith('/?popout=drive&space=x', '_blank')
    open.mockRestore()
  })

  it('says so when the window cannot open', async () => {
    api.Open.mockRejectedValue(new Error('no such surface'))

    usePopOut()({ space: 'x', path: null, session: null })
    await flushPromises()

    expect(useToasts().toasts.value.at(-1)?.severity).toBe('error')
  })
})
