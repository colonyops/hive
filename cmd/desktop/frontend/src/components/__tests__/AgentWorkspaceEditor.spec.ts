import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import AgentWorkspaceEditor from '../AgentWorkspaceEditor.vue'
import { resetAgentWorkspacesForTests, useAgentWorkspaces } from '../../composables/useAgentWorkspaces'
import type {
  AgentSchedule, AgentScheduleRun, AgentWorkspace, MCPCatalogueEntry, SkillPackage,
} from '../../lib/agentWorkspacesClient'

// The schedule half of the form talks to the control plane for three things
// (preview, Run now, and the run history), so those specs install a client. The
// rest run with none, the way the composable leaves it when the probe fails,
// which is what keeps a seeded mcpCatalogue/skillPackages from being read back
// over by an empty response.
const mocks = vi.hoisted(() => ({
  Available: vi.fn(),
  Endpoint: vi.fn(),
  getAgentsEndpoint: vi.fn(),
  client: null as unknown,
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice', () => ({
  Available: mocks.Available,
  Endpoint: mocks.Endpoint,
}))
vi.mock('../../lib/agentWorkspacesClient', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/agentWorkspacesClient')>()),
  getAgentsEndpoint: mocks.getAgentsEndpoint,
  createAgentWorkspacesClient: () => mocks.client,
}))

const demo: AgentWorkspace = {
  dir: 'demo', name: 'Demo', command: 'claude', danger: false, mcps: [], skills: [], schedules: [], problem: '', notice: '',
}

const playwright: MCPCatalogueEntry = {
  id: 'playwright', title: 'Playwright', description: 'Browser automation', shipped: true,
  stability: 'stable', shadows: '', transport: 'stdio', command: 'npx -y @playwright/mcp@latest', problem: '',
}

const hivePackage: SkillPackage = {
  name: 'hive',
  title: 'Hive',
  description: 'Configure Hive Desktop itself.',
  members: [{ slug: 'hive-mcp', shipped: true }, { slug: 'hive-flows', shipped: true }],
}

const infraPackage: SkillPackage = {
  name: 'infra',
  title: 'infra',
  description: 'Terraform and Kubernetes.',
  members: [{ slug: 'terraform-plan', shipped: false }],
}

// The drawer teleports to the body.
function el<T extends HTMLElement>(testid: string): T | null {
  return document.querySelector<T>(`[data-testid="${testid}"]`)
}

function mountEditor(workspace: AgentWorkspace | null = demo) {
  return mount(AgentWorkspaceEditor, {
    props: { workspace },
    attachTo: document.body,
  })
}

function schedule(overrides: Partial<AgentSchedule> = {}): AgentSchedule {
  return {
    id: 'weekly-summary', name: 'Weekly summary', cron: '0 9 * * 5',
    prompt: 'Summarize the week.', disabled: false, onMissed: 'run',
    nextRunAt: null, lastRun: null, ...overrides,
  }
}

function run(overrides: Partial<AgentScheduleRun> = {}): AgentScheduleRun {
  return { id: 3, startedAt: Date.now(), reason: 'due', status: 'launched', missed: 0, error: '', ...overrides }
}

function scheduleClient() {
  return {
    mcpCatalogue: vi.fn().mockResolvedValue([]),
    skillPackages: vi.fn().mockResolvedValue({ packages: [], skills: [], problem: '' }),
    scheduleRuns: vi.fn().mockResolvedValue([run()]),
    runSchedule: vi.fn().mockResolvedValue(run({ reason: 'manual' })),
    previewSchedule: vi.fn().mockResolvedValue({ next: [], cronError: '', promptError: '' }),
  }
}

function typeInto(testid: string, value: string): void {
  const field = el<HTMLInputElement | HTMLTextAreaElement>(testid)!
  field.value = value
  field.dispatchEvent(new Event('input', { bubbles: true }))
}

// AppSelect's popover teleports out of the drawer, so both halves are reached
// through the document rather than the wrapper.
async function chooseOption(wrapper: VueWrapper, testid: string, value: string): Promise<void> {
  el<HTMLButtonElement>(testid)!.click()
  await flushPromises()
  document.querySelector<HTMLElement>(`[data-testid="${testid}-option-${value}"]`)!
    .dispatchEvent(new MouseEvent('click', { bubbles: true }))
  await wrapper.vm.$nextTick()
}

/** Long enough for the card's 300ms preview debounce to fire and land. */
function settlePreview(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 350))
}

function savedSchedules(wrapper: VueWrapper) {
  const saves = wrapper.emitted('save') as unknown[][] | undefined
  return (saves?.[0]?.[0] as { schedules: unknown[] } | undefined)?.schedules
}

function chooseCommand(wrapper: VueWrapper, value: string): Promise<void> {
  return chooseOption(wrapper, 'agent-workspace-editor-command-preset', value)
}

beforeEach(() => {
  document.body.innerHTML = ''
  resetAgentWorkspacesForTests()
  mocks.client = null
  mocks.Available.mockResolvedValue({ available: true, reason: '' })
  mocks.getAgentsEndpoint.mockResolvedValue({ httpBaseURL: 'http://127.0.0.1:1', wsURL: 'ws://127.0.0.1:1/s', token: 'test' })
})

describe('AgentWorkspaceEditor', () => {
  it('keeps delete a quiet footer action that only emits after the inline confirm', async () => {
    const wrapper = mountEditor()
    expect(el('agent-workspace-editor-delete-confirm')).toBeNull()

    el<HTMLButtonElement>('agent-workspace-editor-delete')!.click()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-delete-confirm')).not.toBeNull()
    expect(wrapper.emitted('delete')).toBeUndefined()

    el<HTMLButtonElement>('agent-workspace-editor-delete-confirm-confirm')!.click()
    expect(wrapper.emitted('delete')).toEqual([['demo']])
    wrapper.unmount()
  })

  it('cancelling the confirm strip returns to the form without emitting', async () => {
    const wrapper = mountEditor()
    el<HTMLButtonElement>('agent-workspace-editor-delete')!.click()
    await wrapper.vm.$nextTick()

    el<HTMLButtonElement>('agent-workspace-editor-delete-confirm-cancel')!.click()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-delete-confirm')).toBeNull()
    expect(el('agent-workspace-editor-delete')).not.toBeNull()
    expect(wrapper.emitted('delete')).toBeUndefined()
    wrapper.unmount()
  })

  // ── Unsaved work ────────────────────────────────────────────────────────
  // Every gesture that leaves the sheet used to drop the whole form silently.
  it('closes straight away when nothing was edited', async () => {
    const wrapper = mountEditor()
    await flushPromises()

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(el('agent-workspace-editor-discard-confirm')).toBeNull()
    wrapper.unmount()
  })

  it('asks before Escape throws an edit away, and keeps the sheet open on Keep editing', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    typeInto('agent-workspace-editor-name', 'Demo renamed')
    await wrapper.vm.$nextTick()

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(el('agent-workspace-editor-discard-confirm')).not.toBeNull()

    el<HTMLButtonElement>('agent-workspace-editor-discard-confirm-cancel')!.click()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-discard-confirm')).toBeNull()
    expect(wrapper.emitted('close')).toBeUndefined()
    wrapper.unmount()
  })

  it('closes on Discard', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    typeInto('agent-workspace-editor-name', 'Demo renamed')
    await wrapper.vm.$nextTick()

    el<HTMLButtonElement>('agent-workspace-editor-close')!.click()
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('agent-workspace-editor-discard-confirm-confirm')!.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('asks on the backdrop and on the footer Cancel too, not only on Escape', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    typeInto('agent-workspace-editor-name', 'Demo renamed')
    await wrapper.vm.$nextTick()

    el<HTMLElement>('agent-workspace-editor-backdrop')!.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(el('agent-workspace-editor-discard-confirm')).not.toBeNull()

    el<HTMLButtonElement>('agent-workspace-editor-discard-confirm-cancel')!.click()
    await wrapper.vm.$nextTick()

    el<HTMLButtonElement>('agent-workspace-editor-cancel')!.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(el('agent-workspace-editor-discard-confirm')).not.toBeNull()
    wrapper.unmount()
  })

  // The question is what a save would write, not whether anything was typed.
  it('reads an edit that was typed back to its original as unedited', async () => {
    const wrapper = mountEditor()
    await flushPromises()
    typeInto('agent-workspace-editor-name', 'Demo renamed')
    await wrapper.vm.$nextTick()
    typeInto('agent-workspace-editor-name', 'Demo')
    await wrapper.vm.$nextTick()

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('a pending confirm makes the form inert: no save on Enter, no close on Escape', async () => {
    const wrapper = mountEditor()
    el<HTMLButtonElement>('agent-workspace-editor-delete')!.click()
    await wrapper.vm.$nextTick()

    const name = el<HTMLInputElement>('agent-workspace-editor-name')!
    name.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    expect(wrapper.emitted('save')).toBeUndefined()

    // Escape answers the strip (cancel), never the sheet.
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(el('agent-workspace-editor-delete-confirm')).toBeNull()
    wrapper.unmount()
  })

  it('the delete confirm names the folder it removes from disk', async () => {
    const { root } = useAgentWorkspaces()
    root.value = '/Users/me/workspaces'
    const wrapper = mountEditor()
    await wrapper.vm.$nextTick()

    el<HTMLButtonElement>('agent-workspace-editor-delete')!.click()
    await wrapper.vm.$nextTick()

    const confirm = el<HTMLElement>('agent-workspace-editor-delete-confirm')!
    expect(confirm.textContent).toContain('/Users/me/workspaces/demo')
    expect(confirm.textContent).toContain('deleted from disk')
    wrapper.unmount()
  })

  // A manifest the loader could not read reaches the form as empty lists, so a
  // save would reconcile mcps:/skills:/schedules: to nothing. The Go side
  // refuses the update; the form refuses it first and names the file to fix.
  it('refuses to save a workspace whose manifest could not be read', async () => {
    const wrapper = mountEditor({ ...demo, problem: 'agent-workspace.yaml: line 4: mapping values are not allowed' })
    await wrapper.vm.$nextTick()

    const notice = el('agent-workspace-editor-problem')!
    expect(notice.textContent).toContain('mapping values are not allowed')
    expect(notice.textContent).toContain('Fix agent-workspace.yaml')
    expect(el<HTMLButtonElement>('agent-workspace-editor-save')!.disabled).toBe(true)
    // Deleting a workspace whose file is broken has to stay possible.
    expect(el<HTMLButtonElement>('agent-workspace-editor-delete')!.disabled).toBe(false)
    wrapper.unmount()
  })

  it('creation offers no delete', () => {
    const wrapper = mountEditor(null)
    expect(el('agent-workspace-editor-delete')).toBeNull()
    expect(el('agent-workspace-editor-save')).not.toBeNull()
    wrapper.unmount()
  })

  it('picking a suggestion sets the command, with no template box in the way', async () => {
    const { presets } = useAgentWorkspaces()
    presets.value = [
      { id: 'claude-ask', agent: 'claude', label: 'Ask', command: 'claude --session-id x', danger: false, source: 'builtin' },
      { id: 'claude-full', agent: 'claude', label: 'Full', command: 'claude --dangerously-skip-permissions', danger: true, source: 'builtin' },
    ]
    const wrapper = mountEditor()
    await wrapper.vm.$nextTick()

    await chooseCommand(wrapper, 'claude-full')
    expect(el('agent-workspace-editor-command-input')).toBeNull()
    expect(el<HTMLElement>('agent-workspace-editor-command-preview')!.textContent)
      .toBe('claude --dangerously-skip-permissions')

    el<HTMLButtonElement>('agent-workspace-editor-save')!.click()
    expect(wrapper.emitted('save')).toEqual([[
      { dir: 'demo', name: 'Demo', command: 'claude --dangerously-skip-permissions', mcps: [], skills: [], schedules: [] },
    ]])
    wrapper.unmount()
  })

  // A hive "fable" profile is a claude row, so the mark is the only thing that
  // says which CLI it runs.
  it('a suggestion carries its agent mark, sized to the row', async () => {
    const { presets } = useAgentWorkspaces()
    presets.value = [
      { id: 'hive-fable', agent: 'claude', label: 'fable', command: 'claude --model fable', danger: false, source: 'hive' },
    ]
    const wrapper = mountEditor()
    await wrapper.vm.$nextTick()

    el<HTMLButtonElement>('agent-workspace-editor-command-preset')!.click()
    await wrapper.vm.$nextTick()

    const row = el<HTMLButtonElement>('agent-workspace-editor-command-preset-option-hive-fable')!
    expect(row.textContent).toContain('fable')
    const mark = row.querySelector('svg')
    expect(mark).not.toBeNull()
    expect(mark!.getAttribute('class')).toContain('size-4')
    wrapper.unmount()
  })

  it('custom reveals the template box, seeded with the command already chosen', async () => {
    const { presets } = useAgentWorkspaces()
    presets.value = [
      { id: 'claude-ask', agent: 'claude', label: 'Ask', command: 'claude --session-id x', danger: false, source: 'builtin' },
    ]
    const wrapper = mountEditor()
    await wrapper.vm.$nextTick()

    await chooseCommand(wrapper, 'claude-ask')
    expect(el('agent-workspace-editor-command-fields')).toBeNull()

    await chooseCommand(wrapper, '__custom__')
    const input = el<HTMLTextAreaElement>('agent-workspace-editor-command-input')!
    expect(input.value).toBe('claude --session-id x')
    expect(el<HTMLElement>('agent-workspace-editor-command-fields')!.textContent).toContain('{{ .SessionID }}')

    input.value = 'claude --session-id x --model opus'
    input.dispatchEvent(new Event('input'))
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('agent-workspace-editor-save')!.click()
    expect(wrapper.emitted('save')).toEqual([[
      { dir: 'demo', name: 'Demo', command: 'claude --session-id x --model opus', mcps: [], skills: [], schedules: [] },
    ]])
    wrapper.unmount()
  })

  it('a hand-written command opens the editor on custom', async () => {
    const { presets } = useAgentWorkspaces()
    presets.value = [
      { id: 'claude-ask', agent: 'claude', label: 'Ask', command: 'claude --session-id x', danger: false, source: 'builtin' },
    ]
    const wrapper = mountEditor({ ...demo, command: 'pi --some-flag' })
    await wrapper.vm.$nextTick()

    expect(el<HTMLTextAreaElement>('agent-workspace-editor-command-input')!.value).toBe('pi --some-flag')
    wrapper.unmount()
  })

  // The posture enum used to label its own danger. A free-form command cannot,
  // so the warning is derived from the text — including one typed by hand that
  // no preset offered.
  it('warns about a permission bypass typed into the command', async () => {
    const wrapper = mountEditor()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-command-danger')).toBeNull()

    await chooseCommand(wrapper, '__custom__')
    const input = el<HTMLTextAreaElement>('agent-workspace-editor-command-input')!
    input.value = 'pi --yolo'
    input.dispatchEvent(new Event('input'))
    await wrapper.vm.$nextTick()

    expect(el('agent-workspace-editor-command-danger')).not.toBeNull()
    wrapper.unmount()
  })

  // The whole point of the schema change: an agent this build ships no preset
  // for is editable and saveable, not refused.
  it('saves a command for an agent with no preset', async () => {
    const { presets } = useAgentWorkspaces()
    presets.value = []
    const wrapper = mountEditor()
    await wrapper.vm.$nextTick()

    const input = el<HTMLTextAreaElement>('agent-workspace-editor-command-input')!
    input.value = 'pi --some-flag'
    input.dispatchEvent(new Event('input'))
    await wrapper.vm.$nextTick()

    el<HTMLButtonElement>('agent-workspace-editor-save')!.click()
    expect(wrapper.emitted('save')).toEqual([[
      { dir: 'demo', name: 'Demo', command: 'pi --some-flag', mcps: [], skills: [], schedules: [] },
    ]])
    wrapper.unmount()
  })

  it('save carries the toggled mcps list', async () => {
    const { mcpCatalogue } = useAgentWorkspaces()
    mcpCatalogue.value = [playwright]
    const wrapper = mountEditor()
    await wrapper.vm.$nextTick()

    el<HTMLButtonElement>('agent-workspace-editor-mcp-playwright')!.click()
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('agent-workspace-editor-save')!.click()

    expect(wrapper.emitted('save')).toEqual([[
      { dir: 'demo', name: 'Demo', command: 'claude', mcps: ['playwright'], skills: [], schedules: [] },
    ]])
    wrapper.unmount()
  })

  it('save carries the toggled package list', async () => {
    const { skillPackages } = useAgentWorkspaces()
    skillPackages.value = [hivePackage, infraPackage]
    const wrapper = mountEditor({ ...demo, skills: ['hive'] })
    await wrapper.vm.$nextTick()

    // A package is offered but off until this workspace switches it on — the
    // definitions are shared, the toggle is the workspace's own.
    el<HTMLButtonElement>('agent-workspace-editor-skill-infra')!.click()
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('agent-workspace-editor-save')!.click()

    expect(wrapper.emitted('save')).toEqual([[
      { dir: 'demo', name: 'Demo', command: 'claude', mcps: [], skills: ['hive', 'infra'], schedules: [] },
    ]])
    wrapper.unmount()
  })

  it('a package expands to the skills its patterns select', async () => {
    const { skillPackages } = useAgentWorkspaces()
    skillPackages.value = [hivePackage]
    const wrapper = mountEditor()
    await wrapper.vm.$nextTick()

    const members = el<HTMLButtonElement>('agent-workspace-editor-skill-members-hive')!
    expect(members.textContent).toContain('2 skills')
    expect(members.getAttribute('aria-expanded')).toBe('false')

    const section = el<HTMLElement>('agent-workspace-editor-skills')!
    expect(section.textContent).not.toContain('hive-mcp')

    members.click()
    await wrapper.vm.$nextTick()
    expect(section.textContent).toContain('hive-mcp')
    expect(section.textContent).toContain('hive-flows')
    wrapper.unmount()
  })

  it('an enabled package skills.yml no longer defines still rows, marked missing', async () => {
    const { skillPackages } = useAgentWorkspaces()
    skillPackages.value = [hivePackage]
    const wrapper = mountEditor({ ...demo, skills: ['deleted-package'] })
    await wrapper.vm.$nextTick()

    expect(el('agent-workspace-editor-skill-deleted-package')).not.toBeNull()
    expect(el<HTMLElement>('agent-workspace-editor-skills')!.textContent).toContain('not defined in skills.yml')
    wrapper.unmount()
  })

  it('names the fix when an enabled name is a skill rather than a package', async () => {
    const { skillPackages, skillNames } = useAgentWorkspaces()
    skillPackages.value = [hivePackage]
    skillNames.value = [
      { slug: 'hive-mcp', shipped: true, selectedBy: ['hive'] },
      { slug: 'hive-flows', shipped: true, selectedBy: ['hive'] },
    ]
    const wrapper = mountEditor({ ...demo, skills: ['hive-mcp'] })
    await wrapper.vm.$nextTick()

    const text = el<HTMLElement>('agent-workspace-editor-skills')!.textContent!
    expect(text).toContain('a skill, not a package')
    expect(text).toContain('"hive" package selects it')
    expect(text).not.toContain('not defined in skills.yml')
    wrapper.unmount()
  })

  it('a skill no package selects says to define one rather than pointing nowhere', async () => {
    const { skillPackages, skillNames } = useAgentWorkspaces()
    skillPackages.value = [hivePackage]
    skillNames.value = [{ slug: 'orphan', shipped: false, selectedBy: [] }]
    const wrapper = mountEditor({ ...demo, skills: ['orphan'] })
    await wrapper.vm.$nextTick()

    expect(el<HTMLElement>('agent-workspace-editor-skills')!.textContent).toContain('no package selects it yet')
    wrapper.unmount()
  })

  it('a package whose patterns match nothing says so', async () => {
    const { skillPackages } = useAgentWorkspaces()
    skillPackages.value = [{ name: 'typoed', title: 'typoed', description: '', members: [] }]
    const wrapper = mountEditor()
    await wrapper.vm.$nextTick()

    expect(el<HTMLElement>('agent-workspace-editor-skills')!.textContent).toContain('matches no skill')
    wrapper.unmount()
  })

  it('a declared id the catalogue no longer resolves still rows, marked missing', async () => {
    const { mcpCatalogue } = useAgentWorkspaces()
    mcpCatalogue.value = [playwright]
    const wrapper = mountEditor({ ...demo, mcps: ['ghost'] })
    await wrapper.vm.$nextTick()

    const toggle = el<HTMLButtonElement>('agent-workspace-editor-mcp-ghost')
    expect(toggle).not.toBeNull()
    expect(toggle!.getAttribute('aria-checked')).toBe('true')
    expect(el('agent-workspace-editor-mcp-remove-ghost')).toBeNull()
    wrapper.unmount()
  })

  it('format json pretty-prints the paste box, and reports invalid input', async () => {
    const wrapper = mountEditor()
    el<HTMLButtonElement>('agent-workspace-editor-mcp-import')!.click()
    await wrapper.vm.$nextTick()

    const textarea = el<HTMLTextAreaElement>('agent-workspace-editor-mcp-import-text')!
    textarea.value = '{"a":{"command":"npx"}}'
    textarea.dispatchEvent(new Event('input', { bubbles: true }))
    await wrapper.vm.$nextTick()

    el<HTMLButtonElement>('agent-workspace-editor-mcp-import-format')!.click()
    await wrapper.vm.$nextTick()
    expect(textarea.value).toBe('{\n  "a": {\n    "command": "npx"\n  }\n}')
    expect(el('agent-workspace-editor-mcp-error')).toBeNull()

    textarea.value = 'not json'
    textarea.dispatchEvent(new Event('input', { bubbles: true }))
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('agent-workspace-editor-mcp-import-format')!.click()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-mcp-error')).not.toBeNull()
    wrapper.unmount()
  })

  // ── Schedules ─────────────────────────────────────────────────────────────
  // A schedule is manifest state, so the form holds the whole list and Save
  // sends it. The rows read their cron back as a sentence rather than showing
  // one, and one schedule at a time is edited on a page the sheet turns into.
  it('states each of the workspace manifest schedules as a row', async () => {
    const wrapper = mountEditor({
      ...demo,
      schedules: [schedule({ lastRun: run({ status: 'failed', reason: 'catch_up', error: 'the agent exited' }) })],
    })
    await wrapper.vm.$nextTick()

    expect(el('agent-workspace-editor-schedule-0-summary')!.textContent).toBe('Every Friday at 09:00')
    expect(el('agent-workspace-editor-schedule-0-next')!.textContent).toBe('not scheduled')
    expect(el('agent-workspace-editor-schedule-0-last-run')!.textContent).toContain('Last run failed just now · catch-up')
    expect(el('agent-workspace-editor-schedule-0')!.textContent).toContain('the agent exited')
    expect(el('agent-workspace-editor-schedule-page')).toBeNull()
    wrapper.unmount()
  })

  it('adds a schedule from the page, compiles its shape to cron, and saves the whole list', async () => {
    const wrapper = mountEditor()
    el<HTMLButtonElement>('agent-workspace-editor-schedule-add')!.click()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-page')).not.toBeNull()
    expect(el('agent-workspace-editor-save')).toBeNull()

    typeInto('agent-workspace-editor-schedule-name', 'Weekly summary')
    await wrapper.vm.$nextTick()
    typeInto('agent-workspace-editor-schedule-prompt', 'Summarize the week.')
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('agent-workspace-editor-schedule-keep')!.click()
    await wrapper.vm.$nextTick()

    expect(el('agent-workspace-editor-schedule-page')).toBeNull()
    expect(el('agent-workspace-editor-schedule-0-unsaved')).not.toBeNull()
    expect(el('agent-workspace-editor-schedule-0-next')).toBeNull()

    el<HTMLButtonElement>('agent-workspace-editor-save')!.click()
    expect(savedSchedules(wrapper)).toEqual([{
      id: 'weekly-summary', name: 'Weekly summary', cron: '0 9 * * 5',
      prompt: 'Summarize the week.', disabled: false, onMissed: 'run',
    }])
    wrapper.unmount()
  })

  // The page says what is missing only once Done has been tried, so a fresh
  // page is not red before anything is typed; after that the line follows
  // the fields. The id derives from the name and the Go side upserts by id,
  // so a name that lands on a row's id is refused here too: two rows on one
  // id would silently drop a schedule.
  it('says what stops Done in order: the name, an id collision, the prompt', async () => {
    const wrapper = mountEditor({ ...demo, schedules: [schedule()] })
    el<HTMLButtonElement>('agent-workspace-editor-schedule-add')!.click()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-problem')).toBeNull()

    el<HTMLButtonElement>('agent-workspace-editor-schedule-keep')!.click()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-page')).not.toBeNull()
    expect(el('agent-workspace-editor-schedule-problem')!.textContent).toContain('A name is required.')

    typeInto('agent-workspace-editor-schedule-name', 'Weekly Summary')
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-problem')!.textContent)
      .toContain('Another schedule already uses the id "weekly-summary"')

    typeInto('agent-workspace-editor-schedule-name', 'Nightly digest')
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-problem')!.textContent).toContain('A prompt is required.')

    typeInto('agent-workspace-editor-schedule-prompt', 'Again.')
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('agent-workspace-editor-schedule-keep')!.click()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-page')).toBeNull()
    expect(el('agent-workspace-editor-schedule-1')!.textContent).toContain('Nightly digest')
    wrapper.unmount()
  })

  // Escape steps back off the page rather than out of the sheet, so a stray
  // press costs the draft and nothing else.
  it('drops the draft on Cancel and on Escape without touching the list', async () => {
    const wrapper = mountEditor({ ...demo, schedules: [schedule()] })
    el<HTMLButtonElement>('agent-workspace-editor-schedule-0-edit')!.click()
    await wrapper.vm.$nextTick()
    typeInto('agent-workspace-editor-schedule-name', 'Renamed')
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('agent-workspace-editor-schedule-cancel')!.click()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-page')).toBeNull()
    expect(el('agent-workspace-editor-schedule-0')!.textContent).toContain('Weekly summary')

    el<HTMLButtonElement>('agent-workspace-editor-schedule-add')!.click()
    await wrapper.vm.$nextTick()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-page')).toBeNull()
    expect(el('agent-workspace-editor-schedule-1')).toBeNull()
    expect(wrapper.emitted('close')).toBeUndefined()
    wrapper.unmount()
  })

  // The id keys the run history and the scheduler's cursor, so a saved row
  // keeps it however the name changes; the flags the page is not showing ride
  // the save untouched too. A hand-authored entry may name nothing at all: the
  // row shows its id instead, and the save must not write `name: <id>` back
  // into the manifest on the user's behalf.
  it('keeps a saved id, its unedited flags, and an empty name through the page', async () => {
    const wrapper = mountEditor({
      ...demo,
      schedules: [schedule({ disabled: true, onMissed: 'skip' }), schedule({ id: 'nightly', name: '' })],
    })
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-1')!.textContent).toContain('nightly')

    el<HTMLButtonElement>('agent-workspace-editor-schedule-0-edit')!.click()
    await wrapper.vm.$nextTick()
    typeInto('agent-workspace-editor-schedule-name', 'Monday digest')
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('agent-workspace-editor-schedule-keep')!.click()
    await wrapper.vm.$nextTick()

    el<HTMLButtonElement>('agent-workspace-editor-schedule-1-edit')!.click()
    await wrapper.vm.$nextTick()
    expect(el<HTMLInputElement>('agent-workspace-editor-schedule-name')!.placeholder).toBe('nightly')
    el<HTMLButtonElement>('agent-workspace-editor-schedule-keep')!.click()
    await wrapper.vm.$nextTick()

    el<HTMLButtonElement>('agent-workspace-editor-save')!.click()
    expect(savedSchedules(wrapper)).toEqual([
      {
        id: 'weekly-summary', name: 'Monday digest', cron: '0 9 * * 5',
        prompt: 'Summarize the week.', disabled: true, onMissed: 'skip',
      },
      {
        id: 'nightly', name: '', cron: '0 9 * * 5',
        prompt: 'Summarize the week.', disabled: false, onMissed: 'run',
      },
    ])
    wrapper.unmount()
  })

  // A saved row whose timetable the page changed no longer knows its next
  // run: the scheduler works that out on Save, and the row says so instead of
  // showing the old time.
  it('recompiles the cron as the weekly day chips are picked', async () => {
    const wrapper = mountEditor({ ...demo, schedules: [schedule({ nextRunAt: Date.now() + 3 * 60 * 60 * 1000 })] })
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-0-next')!.textContent).toBe('next in 2h')

    el<HTMLButtonElement>('agent-workspace-editor-schedule-0-edit')!.click()
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('agent-workspace-editor-schedule-day-1')!.click()
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('agent-workspace-editor-schedule-keep')!.click()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-0-summary')!.textContent).toBe('Mon, Fri at 09:00')
    expect(el('agent-workspace-editor-schedule-0-unsaved')).not.toBeNull()
    expect(el('agent-workspace-editor-schedule-0-next')).toBeNull()

    el<HTMLButtonElement>('agent-workspace-editor-save')!.click()
    expect(savedSchedules(wrapper)).toEqual([expect.objectContaining({ cron: '0 9 * * 1,5' })])
    wrapper.unmount()
  })

  // Custom is where an expression no shape can state stays editable. Switching
  // to it carries the compiled cron over, so the box opens on what the picker
  // was already saying.
  it('shows the raw cron under Custom and saves what is typed there', async () => {
    const wrapper = mountEditor({ ...demo, schedules: [schedule()] })
    el<HTMLButtonElement>('agent-workspace-editor-schedule-0-edit')!.click()
    await wrapper.vm.$nextTick()

    await chooseOption(wrapper, 'agent-workspace-editor-schedule-repeat', 'custom')
    expect(el<HTMLInputElement>('agent-workspace-editor-schedule-cron')!.value).toBe('0 9 * * 5')

    typeInto('agent-workspace-editor-schedule-cron', '0 */2 * * *')
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('agent-workspace-editor-schedule-keep')!.click()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-0-summary')!.textContent).toBe('Custom: 0 */2 * * *')

    el<HTMLButtonElement>('agent-workspace-editor-save')!.click()
    expect(savedSchedules(wrapper)).toEqual([expect.objectContaining({ cron: '0 */2 * * *' })])
    wrapper.unmount()
  })

  it('blocks Done on a cron error the preview reports', async () => {
    const client = scheduleClient()
    client.previewSchedule.mockResolvedValue({ next: [], cronError: 'not a cron', promptError: '' })
    mocks.client = client
    const wrapper = mountEditor({ ...demo, schedules: [schedule()] })
    await flushPromises()

    el<HTMLButtonElement>('agent-workspace-editor-schedule-0-edit')!.click()
    await settlePreview()
    await flushPromises()
    expect(client.previewSchedule).toHaveBeenCalledWith({ workspace: 'demo', cron: '0 9 * * 5', prompt: 'Summarize the week.' })

    el<HTMLButtonElement>('agent-workspace-editor-schedule-keep')!.click()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-page')).not.toBeNull()
    expect(el('agent-workspace-editor-schedule-problem')!.textContent).toContain('not a cron')
    wrapper.unmount()
  })

  // The run the Go side answers with is the schedule's newest, so the row
  // shows it as the last run without re-reading the workspace.
  it('runs a schedule now and shows the run it answered with as the last run', async () => {
    const client = scheduleClient()
    mocks.client = client
    const wrapper = mountEditor({ ...demo, schedules: [schedule()] })
    await flushPromises()
    expect(el('agent-workspace-editor-schedule-0-last-run')).toBeNull()

    el<HTMLButtonElement>('agent-workspace-editor-schedule-0-run')!.click()
    await flushPromises()

    expect(client.runSchedule).toHaveBeenCalledWith('demo', 'weekly-summary')
    expect(el('agent-workspace-editor-schedule-0-last-run')!.textContent).toContain('Last run launched just now · by hand')
    wrapper.unmount()
  })

  // A saved schedule's page reads its history as it opens.
  it('loads the run history with the page', async () => {
    const client = scheduleClient()
    mocks.client = client
    const wrapper = mountEditor({ ...demo, schedules: [schedule()] })
    await flushPromises()
    expect(client.scheduleRuns).not.toHaveBeenCalled()

    el<HTMLButtonElement>('agent-workspace-editor-schedule-0-edit')!.click()
    await flushPromises()
    expect(client.scheduleRuns).toHaveBeenCalledWith('demo', 'weekly-summary', 20)
    expect(el('agent-workspace-editor-schedule-history')!.textContent).toContain('launched · on schedule')
    wrapper.unmount()
  })

  it('removes a schedule only after the page\'s inline confirm is answered', async () => {
    const wrapper = mountEditor({ ...demo, schedules: [schedule()] })
    el<HTMLButtonElement>('agent-workspace-editor-schedule-0-edit')!.click()
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('agent-workspace-editor-schedule-remove')!.click()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-page')).not.toBeNull()

    el<HTMLButtonElement>('agent-workspace-editor-schedule-remove-confirm-cancel')!.click()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-remove-confirm')).toBeNull()

    el<HTMLButtonElement>('agent-workspace-editor-schedule-remove')!.click()
    await wrapper.vm.$nextTick()
    el<HTMLButtonElement>('agent-workspace-editor-schedule-remove-confirm-confirm')!.click()
    await wrapper.vm.$nextTick()
    expect(el('agent-workspace-editor-schedule-page')).toBeNull()
    expect(el('agent-workspace-editor-schedule-0')).toBeNull()

    el<HTMLButtonElement>('agent-workspace-editor-save')!.click()
    expect(savedSchedules(wrapper)).toEqual([])
    wrapper.unmount()
  })
})
