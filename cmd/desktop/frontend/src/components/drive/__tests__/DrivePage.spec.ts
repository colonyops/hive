import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import DrivePage from '../DrivePage.vue'
import type { DriveScope } from '../../../lib/drive'
import type { EntryWire } from '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { chooseOption, openSelect } from '../../../test-utils/select'
import { doc, documentFor, folder, root, space } from '../../../test-utils/drive'

const wailsEvents = vi.hoisted(() => ({
  handlers: [] as Array<[string, (event: { data: unknown }) => void]>,
  fire(name: string, data: unknown) {
    for (const [registered, handler] of this.handlers) {
      if (registered === name) handler({ data })
    }
  },
}))
vi.mock('../../../composables/useWailsEvent', () => ({
  useWailsEvent: (name: string, handler: (event: { data: unknown }) => void) => {
    wailsEvents.handlers.push([name, handler])
  },
}))

vi.mock('@wailsio/runtime', () => ({
  Clipboard: { SetText: vi.fn().mockResolvedValue(undefined) },
  Dialogs: { SaveFile: vi.fn().mockResolvedValue('') },
}))

const drive = vi.hoisted(() => ({
  Spaces: vi.fn(),
  Tree: vi.fn(),
  Search: vi.fn(),
  Document: vi.fn(),
  Markdown: vi.fn(),
  Export: vi.fn(),
  Delete: vi.fn(),
}))
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/driveservice', () => drive)
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/windowservice', () => ({
  Focused: vi.fn().mockResolvedValue(true),
}))
const settingsBindings = vi.hoisted(() => ({ AppearanceSettings: vi.fn() }))
vi.mock(
  '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice',
  () => settingsBindings,
)

const plan = doc('plans/2026-10-11-plan.md', {
  title: 'E2E Drive Plan',
  front: { type: 'plan', title: 'E2E Drive Plan' },
  modified: '2026-10-11T00:00:00Z',
})
const report = doc('canvases/report.canvas.json', { title: 'E2E Report', modified: '2026-10-01T00:00:00Z' })
const notes = doc('notes.md', { title: 'Notes', modified: '2026-10-02T00:00:00Z' })
const trees: Record<string, EntryWire> = {
  'e2e/repo': root([folder('canvases', [report]), folder('plans', [plan])]),
  shared: root([notes]),
  'workspaces/demo': root([]),
}

// The view is controlled: AppDialogs owns the scope and hands back each change.
async function mountPage(initial: Partial<DriveScope> = {}) {
  const scope = ref<DriveScope>({ space: 'e2e/repo', path: null, session: null, ...initial })
  const host = defineComponent({
    emits: ['close', 'open-url'],
    setup(_, { emit }) {
      return () =>
        h(DrivePage, {
          scope: scope.value,
          'onUpdate:scope': (next: DriveScope) => (scope.value = next),
          onClose: () => emit('close'),
          onOpenUrl: (url: string) => emit('open-url', url),
        })
    },
  })
  const wrapper = mount(host, { attachTo: document.body })
  await flushPromises()
  return { wrapper, scope }
}

describe('DrivePage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    wailsEvents.handlers = []
    localStorage.clear()
    document.body.innerHTML = ''
    settingsBindings.AppearanceSettings.mockResolvedValue({ canvasFontSize: '', canvasLineSpacing: '' })
    drive.Spaces.mockResolvedValue([
      space('shared', 'Shared'),
      space('workspaces/demo', 'demo'),
      space('e2e/repo', 'e2e/repo'),
    ])
    drive.Tree.mockImplementation((key: string) => Promise.resolve({ ...(trees[key] ?? root([])) }))
    drive.Document.mockImplementation((key: string, path: string) => {
      if (path === plan.path)
        return Promise.resolve(
          documentFor(key, plan, {
            markdown:
              '---\ntype: plan\n---\n\n# E2E Drive Plan\n\nThe [report canvas](../canvases/report.canvas.json) and [the PR](https://example.com/pr/1).',
          }),
        )
      if (path === report.path)
        return Promise.resolve(
          documentFor(key, report, {
            canvas: {
              title: 'E2E Report',
              blocks: [
                { id: 'summary', kind: 'markdown', title: '', body: 'Summary.', url: '', createdAt: 1, updatedAt: 1 },
              ],
            },
          }),
        )
      return Promise.resolve(documentFor(key, notes, { markdown: '# Notes' }))
    })
    drive.Search.mockResolvedValue([{ space: space('e2e/repo'), entry: plan }])
  })

  it("shows the space's tree beside its newest document", async () => {
    const { wrapper } = await mountPage()

    expect(drive.Document).toHaveBeenCalledWith('e2e/repo', 'plans/2026-10-11-plan.md')
    expect(wrapper.get('[data-testid="drive-page-title"]').text()).toBe('E2E Drive Plan')
    expect(wrapper.get('[data-testid="drive-page-title"]').attributes('title')).toBe('plans/2026-10-11-plan.md')
    // The folder only: the title names the document already.
    expect(wrapper.get('[data-testid="drive-page-folder"]').text()).toMatch(/^plans( · .+)?$/)
    // The picker heads the sidebar rather than the title strip.
    expect(wrapper.find('[data-testid="drive-page-sidebar"] [data-testid="drive-page-space"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="drive-page-markdown"] h1').text()).toBe('E2E Drive Plan')
    expect(wrapper.get('[data-testid="drive-page-frontmatter-type"]').text()).toContain('plan')
    expect(wrapper.get('[data-testid="drive-page-row-plans/2026-10-11-plan.md"]').attributes('aria-selected')).toBe(
      'true',
    )
  })

  it('lays the document out at the narrow measure until the setting says otherwise', async () => {
    const narrow = await mountPage()
    expect(narrow.wrapper.get('[data-testid="drive-page-measure"]').classes()).toContain('max-w-3xl')
    narrow.wrapper.unmount()
  })

  it('lets the full-page width setting widen the measure', async () => {
    settingsBindings.AppearanceSettings.mockResolvedValue({
      canvasFontSize: '',
      canvasLineSpacing: '',
      canvasPageWidth: 'full',
    })
    const { wrapper } = await mountPage()
    expect(wrapper.get('[data-testid="drive-page-measure"]').classes()).toContain('max-w-none')
  })

  it('opens on the document it was asked for, and moves from the tree without leaving the view', async () => {
    const { wrapper, scope } = await mountPage({ path: report.path })
    expect(wrapper.get('[data-testid="drive-page-block-summary"]').text()).toContain('Summary.')

    await wrapper.get('[data-testid="drive-page-row-plans"]').trigger('click')
    await wrapper.get('[data-testid="drive-page-row-plans/2026-10-11-plan.md"]').trigger('click')
    await flushPromises()
    expect(scope.value).toEqual({ space: 'e2e/repo', path: 'plans/2026-10-11-plan.md', session: null })
    expect(wrapper.get('[data-testid="drive-page-title"]').text()).toBe('E2E Drive Plan')
  })

  it('opens a link to another document in place and a web link outside the app', async () => {
    const { wrapper, scope } = await mountPage()
    const [toReport, toPr] = wrapper.findAll('[data-testid="drive-page-markdown"] a')
    await toPr.trigger('click')
    expect(wrapper.emitted('open-url')).toEqual([['https://example.com/pr/1']])

    await toReport.trigger('click')
    await flushPromises()
    expect(scope.value.path).toBe('canvases/report.canvas.json')
  })

  it('lists the spaces grouped Repositories, Workspaces, Shared and switches to one', async () => {
    const { wrapper, scope } = await mountPage()
    const popover = await openSelect(wrapper, 'drive-page-space')
    expect(
      Array.from(popover.querySelectorAll('[data-testid^="drive-page-space-group-"]')).map((h) =>
        h.textContent?.trim(),
      ),
    ).toEqual(['Repositories', 'Workspaces', 'Shared'])
    expect(
      Array.from(popover.querySelectorAll('[role="option"] button')).map((option) =>
        option.getAttribute('data-testid'),
      ),
    ).toEqual([
      'drive-page-space-option-e2e/repo',
      'drive-page-space-option-workspaces/demo',
      'drive-page-space-option-shared',
    ])

    // A long listing is found by typing: the search field filters the options.
    const search = popover.querySelector<HTMLInputElement>('[data-testid="drive-page-space-search"]')!
    search.value = 'demo'
    search.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    expect(
      Array.from(popover.querySelectorAll('[role="option"] button')).map((option) =>
        option.getAttribute('data-testid'),
      ),
    ).toEqual(['drive-page-space-option-workspaces/demo'])
    await wrapper.get('[data-testid="drive-page-space"]').trigger('click')

    await chooseOption(wrapper, 'drive-page-space', 'shared')
    await flushPromises()
    expect(scope.value).toEqual({ space: 'shared', path: null, session: null })
    expect(drive.Document).toHaveBeenLastCalledWith('shared', 'notes.md')
  })

  it('searches every space and opens a hit in its own space', async () => {
    const { wrapper, scope } = await mountPage({ space: 'shared' })
    await wrapper.get('[data-testid="drive-page-search"]').setValue('plan')
    await vi.waitFor(() => expect(drive.Search).toHaveBeenCalledWith('plan', 50))
    await flushPromises()

    await wrapper.get('[data-testid="drive-page-hit-e2e/repo:plans/2026-10-11-plan.md"]').trigger('click')
    await flushPromises()
    expect(scope.value).toEqual({ space: 'e2e/repo', path: 'plans/2026-10-11-plan.md', session: null })
    expect(wrapper.get('[data-testid="drive-page-title"]').text()).toBe('E2E Drive Plan')
  })

  it('falls back to the first space when opened with none', async () => {
    const { wrapper } = await mountPage({ space: '' })
    expect(drive.Tree).toHaveBeenCalledWith('shared')
    expect(wrapper.get('[data-testid="drive-page-title"]').text()).toBe('Notes')
  })

  it('says so when there are no spaces at all', async () => {
    drive.Spaces.mockResolvedValue([])
    const { wrapper } = await mountPage({ space: '' })
    expect(wrapper.find('[data-testid="drive-page-no-spaces"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="drive-page-sidebar"]').exists()).toBe(false)
  })

  it('offers agent setup for an empty repository space, and not for a workspace', async () => {
    trees['acme/site'] = root([])
    const repo = await mountPage({ space: 'acme/site' })
    expect(repo.wrapper.find('[data-testid="drive-page-setup"]').exists()).toBe(true)
    repo.wrapper.unmount()

    const workspace = await mountPage({ space: 'workspaces/demo' })
    expect(workspace.wrapper.find('[data-testid="drive-page-setup"]').exists()).toBe(false)
  })

  // Rendering a large document is the expensive part of the page, so a
  // reload that leaves the document's entry as it was keeps what is on screen.
  it('re-reads on drive:updated only when the shown document changed', async () => {
    await mountPage()
    const reads = drive.Document.mock.calls.length
    wailsEvents.fire('drive:updated', { space: 'e2e/repo', paths: [report.path], session: '', hiveSession: '' })
    await flushPromises()
    expect(drive.Document.mock.calls.length).toBe(reads)

    const original = trees['e2e/repo']
    const rewritten = doc(plan.path, { ...plan, modified: '2026-10-12T00:00:00Z' })
    trees['e2e/repo'] = root([folder('canvases', [report]), folder('plans', [rewritten])])
    try {
      wailsEvents.fire('drive:updated', { space: 'e2e/repo', paths: [plan.path], session: '', hiveSession: '' })
      await flushPromises()
      expect(drive.Document.mock.calls.length).toBe(reads + 1)
    } finally {
      trees['e2e/repo'] = original
    }
  })

  it('closes from its button and on Escape', async () => {
    const { wrapper } = await mountPage()
    await wrapper.get('[data-testid="drive-page-close"]').trigger('click')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toHaveLength(2)
  })
})
