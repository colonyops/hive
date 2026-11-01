import type { Locator, Page } from '@playwright/test'
import { screenshotPath } from '../../../fixtures/paths.js'
import { expect, tag, test } from '../../../fixtures/test.js'
import { bindingID } from '../../../fixtures/wails.js'

// Server builds cannot run terminals, so the terminal control plane is mocked and
// the tabs, the split and the drags are real. perf/session-tabs-scrollback.perf.ts
// checks what a drag does to a real terminal's scrollback.
function windowState(windowId: string, paneId: string, name: string, active: boolean) {
  return {
    windowId, name, active, activePane: paneId, width: 100, height: 30,
    layout: { paneId, x: 0, y: 0, width: 100, height: 30, cells: [] },
  }
}
const WINDOWS = [windowState('@1', '%1', 'agent', true), windowState('@2', '%2', 'zsh', false)]
const session = (id: string, name: string) => ({
  id, name, slug: id, repo: 'git@github.com:acme/site.git', repoIdentity: 'git@github.com:acme/site.git',
  server: '', state: 'active', canvasOwner: 'acme/site', tags: [], createdAt: '2026-10-01T12:00:00Z',
})

async function fixture(page: Page) {
  const availability = ['terminalservice', 'agentsservice', 'popupterminalservice'].map((name) => bindingID(name, 'Available'))
  const answers: Record<number, unknown> = {
    [bindingID('sessionservice', 'ListSessions')]: [session('review', 'Review'), session('other', 'Other')],
    [bindingID('terminalservice', 'Scratch')]: { slug: '', name: '' },
    [bindingID('sessionservice', 'SessionChanges')]: {
      resolved: true, branch: 'feat/review', defaultBranch: 'main', base: 'branch', branchAvailable: true,
      truncated: false, against: '', commit: '', commits: [], files: [],
    },
    [bindingID('sessionservice', 'SessionBranches')]: ['refs/heads/main'],
    [bindingID('sessionservice', 'SessionFileDiffs')]: [],
    [bindingID('sessionservice', 'SessionGitStatus')]: {
      path: '/w/review', branch: 'feat/review', dirty: true, unpushed: true, additions: 42, deletions: 7,
      host: 'github.com', owner: 'acme', repo: 'site', resolved: true, error: '',
    },
    [bindingID('sessionservice', 'SessionPullRequest')]: { status: 'none' },
  }
  await page.route('**/wails/runtime', async (route) => {
    const id = route.request().postDataJSON()?.args?.methodID as number | undefined
    if (availability.includes(id as number)) return route.fulfill({ json: { available: true, reason: '' } })
    if (id !== undefined && id in answers) return route.fulfill({ json: answers[id] })
    return route.fallback()
  })
  await page.route('**/api/terminal/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace('/api/terminal/', '')
    const headers = {
      'Access-Control-Allow-Origin': route.request().headers().origin ?? '*',
      'Access-Control-Allow-Headers': 'Authorization, Content-Type',
      'Access-Control-Allow-Methods': 'POST, OPTIONS',
    }
    if (route.request().method() === 'OPTIONS') return route.fulfill({ status: 204, headers })
    let answer: unknown = {}
    if (path === 'attach') answer = { windows: WINDOWS }
    else if (path === 'windows/list') answer = { sessions: { review: WINDOWS, other: WINDOWS } }
    else if (path === 'agents/sessions' || path === 'agents/sessions/all') answer = { sessions: [] }
    else if (path === 'agents/sessions/activity') answer = { items: [] }
    else if (path === 'agents/workspaces') answer = { root: '/w', available: true, workspaces: [], presets: [], editor: { command: '', title: '' } }
    else return route.fulfill({ status: 204, headers })
    await route.fulfill({ json: answer, headers })
  })
  await page.routeWebSocket('**/api/terminal/stream?*', () => {})
  await page.goto('/#/terminal/review')
  await expect(page.getByTestId('session-tab-window:@1')).toBeVisible()
  await expect(page.getByTestId('terminal-window-row').first()).toBeVisible()
  await dismissSizeBanner(page)
}

// The mocked windows stay 100x30 whatever size the panes ask for, which raises the
// tmux size banner 750ms after the last resize. It pushes the tab rows down 55px, so
// a coordinate read before it lands misses after, and a drag can straddle the shift.
async function dismissSizeBanner(page: Page) {
  await page.getByTestId('terminal-size-constraint-dismiss').click()
  await expect(page.getByTestId('terminal-size-constraint')).toHaveCount(0)
}

async function addTab(page: Page, pane: string, label: string) {
  await page.getByTestId(`session-tab-add-${pane}`).click()
  await page.getByTestId(`new-window-view-${label.toLowerCase()}`).click()
}

function tabIds(page: Page, pane: string): Promise<string[]> {
  return page
    .getByTestId(`session-tab-row-${pane}`)
    .locator('[data-session-tab]')
    .evaluateAll((tabs) => tabs.map((tab) => tab.getAttribute('data-testid')!.replace('session-tab-', '')))
}

async function center(target: Locator) {
  const box = (await target.boundingBox())!
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 }
}

// A real pointer gesture: press, travel in steps, release.
async function dragTo(page: Page, from: { x: number; y: number }, to: { x: number; y: number }) {
  await page.mouse.move(from.x, from.y)
  await page.mouse.down()
  await page.mouse.move(to.x, to.y, { steps: 10 })
  await page.mouse.up()
}

async function dragTabBefore(page: Page, tab: string, before: string) {
  const target = (await page.getByTestId(`session-tab-${before}`).boundingBox())!
  await dragTo(page, await center(page.getByTestId(`session-tab-${tab}`)), { x: target.x + 2, y: target.y + target.height / 2 })
}

const paneBox = async (page: Page, pane: string) => (await page.getByTestId(`session-pane-${pane}`).boundingBox())!

test.describe('session tabs', { tag: [tag.terminal] }, () => {
  test('opens each tab once, and restores a session after another', async ({ page, browserName }) => {
    await fixture(page)
    await addTab(page, 'p0', 'Drive')
    await addTab(page, 'p0', 'Diff')
    await addTab(page, 'p0', 'Tasks')
    await addTab(page, 'p0', 'Diff')
    expect(await tabIds(page, 'p0')).toEqual(['window:@1', 'window:@2', 'docs', 'diff', 'tasks'])
    await expect(page.getByTestId('session-tab-diff')).toHaveAttribute('data-shown', 'true')
    await expect(page.getByTestId('changes-view')).toBeVisible()
    await expect(page.getByTestId('titlebar-toggle-preview')).toBeDisabled()
    await page.screenshot({ path: screenshotPath(`session-tabs-tabs-${browserName}.png`) })

    await page.locator('[data-testid="terminal-session-row"][data-slug="other"]').click()
    await expect(page.getByTestId('session-tab-window:@1')).toHaveAttribute('data-shown', 'true')
    expect(await tabIds(page, 'p0')).toEqual(['window:@1', 'window:@2'])

    await page.locator('[data-testid="terminal-session-row"][data-slug="review"]').click()
    await expect(page.getByTestId('session-tab-diff')).toHaveAttribute('data-shown', 'true')
    expect(await tabIds(page, 'p0')).toEqual(['window:@1', 'window:@2', 'docs', 'diff', 'tasks'])
  })

  test('opens Diff from the counts in the status bar', async ({ page }) => {
    await fixture(page)
    await page.getByTestId('session-status-diff').click()
    await expect(page.getByTestId('changes-view')).toBeVisible()
    await expect(page.getByTestId('session-status-host')).toHaveText('Local')
  })

  test('splits through the picker, and puts Terminal beside Diff', async ({ page, browserName }) => {
    await fixture(page)
    await page.getByTestId('session-split-row-p0').click()
    const picker = page.getByTestId('session-pane-picker')
    await expect(picker).toContainText('What goes here?')
    await expect(page.getByTestId('session-pane-picker-window-@2')).toBeVisible()
    await page.screenshot({ path: screenshotPath(`session-tabs-picker-${browserName}.png`) })

    await page.getByTestId('session-pane-picker-view-diff').click()
    await expect(picker).toBeHidden()
    await expect(page.getByTestId('changes-view')).toBeVisible()
    await expect(page.locator('[data-testid="terminal-pane"][data-window-id="@1"]')).toBeVisible()
    await page.screenshot({ path: screenshotPath(`session-tabs-terminal-beside-diff-${browserName}.png`) })

    await page.getByTestId('session-tab-close-diff').click()
    await expect(page.getByTestId('session-pane-p1')).toHaveCount(0)
  })

  test('shows a second window beside the first, and a window keeps one tab', async ({ page }) => {
    await fixture(page)
    await page.getByTestId('session-split-row-p0').click()
    await page.getByTestId('session-pane-picker-window-@2').click()
    await expect(page.locator('[data-testid="terminal-pane"][data-window-id="@1"]')).toBeVisible()
    await expect(page.locator('[data-testid="terminal-pane"][data-window-id="@2"]')).toBeVisible()
    expect(await tabIds(page, 'p0')).toEqual(['window:@1'])
    expect(await tabIds(page, 'p1')).toEqual(['window:@2'])

    await page.getByTestId('session-pane-close-p1').click()
    await expect(page.getByTestId('session-pane-p1')).toHaveCount(0)
    expect(await tabIds(page, 'p0')).toEqual(['window:@1', 'window:@2'])
  })

  test('mirrors the tabs in the tree, one row each and in the same order', async ({ page, browserName }) => {
    await fixture(page)
    await addTab(page, 'p0', 'Drive')
    await addTab(page, 'p0', 'Diff')
    const tree = () =>
      page
        .locator('[data-testid="terminal-session-row"][data-slug="review"]')
        .locator('xpath=ancestor::*[.//*[@data-testid="terminal-window-row"]][1]')
        .locator('[data-testid="terminal-window-row"], [data-testid="terminal-view-row"]')
        .evaluateAll((rows) =>
          rows.map((row) => row.getAttribute('data-window-id') ?? row.getAttribute('data-view-kind')),
        )
    const tabsAsTree = async () => {
      const tabs = [...(await tabIds(page, 'p0')), ...(await tabIds(page, 'p1').catch(() => []))]
      return tabs.map((tab) => tab.replace('window:', ''))
    }

    await expect.poll(tree).toEqual(await tabsAsTree())
    await page.screenshot({ path: screenshotPath(`session-tabs-mirror-${browserName}.png`) })

    await dragTabBefore(page, 'window:@2', 'window:@1')
    await expect.poll(() => tabIds(page, 'p0')).toEqual(['window:@2', 'window:@1', 'docs', 'diff'])
    await expect.poll(tree).toEqual(['@2', '@1', 'docs', 'diff'])

    await dragTabBefore(page, 'diff', 'window:@1')
    await expect.poll(tree).toEqual(['@2', 'diff', '@1', 'docs'])
    await expect.poll(tree).toEqual(await tabsAsTree())

    await page.locator('[data-testid="terminal-window-row"][data-window-id="@1"]').click()
    await expect(page.getByTestId('session-tab-window:@1')).toHaveAttribute('data-shown', 'true')
  })

  test('offers the same + menu on a session row and on a tab row', async ({ page }) => {
    await fixture(page)
    const items = () =>
      page
        .getByTestId('new-window-menu')
        .locator('[data-testid^="new-window-"]')
        .evaluateAll((rows) => rows.map((row) => row.getAttribute('data-testid')))

    await page.getByTestId('session-tab-add-p0').click()
    const fromTabs = await items()
    expect(fromTabs).toEqual(expect.arrayContaining(['new-window-terminal', 'new-window-view-docs', 'new-window-view-diff', 'new-window-view-tasks']))
    await page.keyboard.press('Escape')

    const row = page.locator('[data-testid="terminal-session-row"][data-slug="review"]')
    await row.hover()
    await row.getByTestId('terminal-new-window').click()
    expect(await items()).toEqual(fromTabs)

    await page.getByTestId('new-window-view-tasks').click()
    await expect(page.getByTestId('session-tab-tasks')).toHaveAttribute('data-shown', 'true')
  })

  test('drags tabs along a row and between panes', async ({ page, browserName }) => {
    await fixture(page)
    await addTab(page, 'p0', 'Drive')
    await addTab(page, 'p0', 'Diff')
    await addTab(page, 'p0', 'Tasks')

    await dragTabBefore(page, 'tasks', 'docs')
    await expect.poll(() => tabIds(page, 'p0')).toEqual(['window:@1', 'window:@2', 'tasks', 'docs', 'diff'])

    await dragTabBefore(page, 'diff', 'window:@1')
    await expect.poll(() => tabIds(page, 'p0')).toEqual(['diff', 'window:@1', 'window:@2', 'tasks', 'docs'])

    await page.getByTestId('session-split-row-p0').click()
    await page.getByTestId('session-pane-picker-window-@2').click()
    const row = (await page.getByTestId('session-tab-row-p1').boundingBox())!
    await dragTo(page, await center(page.getByTestId('session-tab-diff')), { x: row.x + row.width / 2, y: row.y + row.height / 2 })
    await expect.poll(() => tabIds(page, 'p1')).toEqual(['window:@2', 'diff'])
    expect(await tabIds(page, 'p0')).toEqual(['window:@1', 'tasks', 'docs'])
    await expect(page.getByTestId('changes-view')).toBeVisible()
    await page.screenshot({ path: screenshotPath(`session-tabs-drag-${browserName}.png`) })

    // Over a terminal rather than a row, which would swallow the pointer without capture.
    await page.getByTestId('session-tab-window:@1').click()
    const terminal = await center(page.locator('[data-testid="terminal-pane"]:visible').first())
    await dragTo(page, await center(page.getByTestId('session-tab-window:@2')), terminal)
    await expect.poll(() => tabIds(page, 'p0')).toContain('window:@2')

    await page.reload()
    await expect(page.getByTestId('session-tab-window:@1')).toBeVisible()
    expect(await tabIds(page, 'p1')).toEqual(['diff'])
  })

  test('lists Drive, Diff and Tasks in the tree', async ({ page, browserName }) => {
    await fixture(page)
    const rows = page.getByTestId('terminal-view-row')
    await expect(rows).toHaveCount(0)

    await addTab(page, 'p0', 'Drive')
    await addTab(page, 'p0', 'Diff')
    await expect(rows).toHaveCount(2)
    await expect(page.locator('[data-testid="terminal-view-row"][data-view-kind="docs"]')).toContainText('Drive')
    await page.screenshot({ path: screenshotPath(`session-tabs-tree-${browserName}.png`) })

    await page.locator('[data-testid="terminal-view-row"][data-view-kind="docs"]').click()
    await expect(page.getByTestId('session-tab-docs')).toHaveAttribute('data-shown', 'true')

    const diffRow = page.locator('[data-testid="terminal-view-row"][data-view-kind="diff"]')
    await diffRow.hover()
    await diffRow.getByTestId('terminal-close-view').click()
    await expect(page.getByTestId('session-tab-diff')).toHaveCount(0)
    await expect(rows).toHaveCount(1)

    await page.getByTestId('session-tab-close-docs').click()
    await expect(rows).toHaveCount(0)
  })

  test('splits within splits, either way, and restores them', async ({ page, browserName }) => {
    await fixture(page)
    await page.getByTestId('session-split-row-p0').click()
    await page.getByTestId('session-pane-picker-view-tasks').click()
    let [first, second] = [await paneBox(page, 'p0'), await paneBox(page, 'p1')]
    expect(second.x).toBeGreaterThan(first.x + first.width - 2)
    expect(second.y).toBe(first.y)
    await page.screenshot({ path: screenshotPath(`session-tabs-side-by-side-${browserName}.png`) })

    // Within the right side, as tmux would.
    await page.getByTestId('session-split-column-p1').click()
    await page.getByTestId('session-pane-picker-window-@2').click()
    const [right, below] = [await paneBox(page, 'p1'), await paneBox(page, 'p2')]
    expect(below.y).toBeGreaterThan(right.y + right.height - 2)
    expect(below.x).toBe(right.x)
    await expect(page.locator('[data-testid="terminal-pane"][data-window-id="@2"]')).toBeVisible()
    await page.screenshot({ path: screenshotPath(`session-tabs-nested-${browserName}.png`) })

    // And within the left side, so three levels sit side by side.
    await page.getByTestId('session-split-column-p0').click()
    await expect(page.getByTestId('session-pane-p3')).toBeVisible()
    await page.getByTestId('session-pane-picker-cancel').click()
    await expect(page.getByTestId('session-pane-p3')).toHaveCount(0)

    // One split turns; the others stay.
    const divider = page.getByTestId('session-split-divider-s2')
    await divider.hover()
    await page.getByTestId('session-split-turn-s2').click()
    const [turnedA, turnedB] = [await paneBox(page, 'p1'), await paneBox(page, 'p2')]
    expect(turnedB.x).toBeGreaterThan(turnedA.x)
    expect(turnedB.y).toBe(turnedA.y)
    await page.screenshot({ path: screenshotPath(`session-tabs-stacked-${browserName}.png`) })

    await page.reload()
    await expect(page.getByTestId('session-pane-p2')).toBeVisible()
    expect((await paneBox(page, 'p2')).x).toBeGreaterThan((await paneBox(page, 'p1')).x)
    expect(await tabIds(page, 'p1')).toEqual(['tasks'])

    ;[first, second] = [await paneBox(page, 'p0'), await paneBox(page, 'p1')]
    expect(second.x).toBeGreaterThan(first.x)

    await page.locator('[data-testid="terminal-session-row"][data-slug="other"]').click()
    await expect(page.getByTestId('session-pane-p1')).toHaveCount(0)
  })

  test('drags a Drive, Diff or Tasks row in the tree, and the tabs follow', async ({ page, browserName }) => {
    await fixture(page)
    await addTab(page, 'p0', 'Drive')
    await addTab(page, 'p0', 'Diff')
    const docs = page.locator('[data-testid="terminal-view-row"][data-view-kind="docs"]')
    const window1 = page.locator('[data-testid="terminal-window-row"][data-window-id="@1"]')
    await expect(docs).toBeVisible()
    // Playwright's dragTo runs the HTML5 drag events this tree uses.
    await docs.dragTo(window1, { targetPosition: { x: 20, y: 2 } })
    await expect.poll(() => tabIds(page, 'p0')).toEqual(['docs', 'window:@1', 'window:@2', 'diff'])
    await page.screenshot({ path: screenshotPath(`session-tabs-tree-drag-${browserName}.png`) })
  })

  test('moves the tree rail to a view when a view has the focus', async ({ page }) => {
    await fixture(page)
    await addTab(page, 'p0', 'Diff')
    const diffRow = page.locator('[data-testid="terminal-view-row"][data-view-kind="diff"]')
    await expect(diffRow).toHaveAttribute('data-active', 'true')
    await expect(page.locator('[data-testid="terminal-window-row"][data-active="true"]')).toHaveCount(0)
    const row = (await diffRow.boundingBox())!
    // The rail slides to its row, so it arrives a moment after the row is active.
    await expect
      .poll(async () => Math.abs((await page.getByTestId('terminal-window-rail').boundingBox())!.y - row.y))
      .toBeLessThan(row.height)

    await page.getByTestId('session-tab-window:@1').click()
    await expect(diffRow).toHaveAttribute('data-active', 'false')
    await expect(page.locator('[data-testid="terminal-window-row"][data-window-id="@1"]')).toHaveAttribute('data-active', 'true')
  })

  test('embeds the global Drive view in the tab', async ({ page, browserName }) => {
    await fixture(page)
    await addTab(page, 'p0', 'Drive')
    await expect(page.getByTestId('drive-page')).toBeVisible()
    await expect(page.getByTestId('drive-pane')).toHaveCount(0)
    await expect(page.getByTestId('drive-page-sidebar')).toBeVisible()
    await expect(page.getByTestId('drive-page-close')).toHaveCount(0)
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('drive-page')).toBeVisible()
    await page.screenshot({ path: screenshotPath(`session-tabs-docs-${browserName}.png`) })

    await page.getByTestId('drive-page-sidebar-toggle').click()
    await expect(page.getByTestId('drive-page-sidebar')).toBeHidden()
  })

  test('brings tabs back after a restart', async ({ page }) => {
    await fixture(page)
    await addTab(page, 'p0', 'Drive')
    await addTab(page, 'p0', 'Diff')
    await page.reload()
    await expect(page.getByTestId('session-tab-diff')).toHaveAttribute('data-shown', 'true')
    expect(await tabIds(page, 'p0')).toEqual(['window:@1', 'window:@2', 'docs', 'diff'])
  })

  test('keeps every view usable at half width and at the clamp', async ({ page, browserName }) => {
    test.setTimeout(60_000)
    await fixture(page)
    await page.getByTestId('session-split-row-p0').click()
    await page.getByTestId('session-pane-picker-view-tasks').click()
    await addTab(page, 'p1', 'Drive')
    await addTab(page, 'p1', 'Diff')

    const show = (kind: string) => page.getByTestId(`session-tab-${kind}`).click()
    const shots = async (width: string) => {
      for (const kind of ['tasks', 'docs', 'diff']) {
        await show(kind)
        await page.screenshot({ path: screenshotPath(`session-tabs-width-${kind}-${width}-${browserName}.png`) })
      }
    }
    const tasksFit = async () => {
      await show('tasks')
      const pane = await paneBox(page, 'p1')
      await expect
        .poll(async () => (await page.getByTestId('tasks-list-column').boundingBox())?.width ?? 0)
        .toBeGreaterThan(pane.width * 0.9)
      await expect(page.getByTestId('tasks-empty')).toContainText('No tasks in')
      const text = (await page.getByTestId('tasks-empty').locator('div').first().boundingBox())!
      expect(text.x + text.width).toBeLessThanOrEqual(pane.x + pane.width)
      const button = (await page.getByTestId('tasks-empty-show-all').boundingBox())!
      expect(button.x).toBeGreaterThanOrEqual(pane.x)
      expect(button.x + button.width).toBeLessThanOrEqual(pane.x + pane.width)
      const filters = (await page.getByTestId('tasks-filters').boundingBox())!
      const filter = (await page.getByTestId('tasks-filter').boundingBox())!
      expect(filter.x + filter.width).toBeLessThanOrEqual(filters.x + filters.width)
    }

    await tasksFit()
    expect((await page.getByTestId('tasks-list-column').boundingBox())!.width).toBeGreaterThan(320)
    await shots('half')

    const divider = await center(page.getByTestId('session-split-divider-s1'))
    await dragTo(page, divider, { x: 5000, y: divider.y })
    expect((await paneBox(page, 'p1')).width).toBeLessThan(260)
    await tasksFit()
    await shots('clamp')
  })

  test('drags the boundary to resize both ways, keeps it, and clamps it', async ({ page, browserName }) => {
    await fixture(page)
    await page.getByTestId('session-split-row-p0').click()
    await page.getByTestId('session-pane-picker-window-@2').click()
    const terminal = await page.locator('[data-testid="terminal-pane"][data-window-id="@1"]').elementHandle()

    const before = await paneBox(page, 'p0')
    const divider = await center(page.getByTestId('session-split-divider-s1'))
    await dragTo(page, divider, { x: divider.x + 180, y: divider.y })
    expect((await paneBox(page, 'p0')).width).toBeCloseTo(before.width + 180, -1)
    await page.screenshot({ path: screenshotPath(`session-tabs-resize-${browserName}.png`) })

    const far = await center(page.getByTestId('session-split-divider-s1'))
    await dragTo(page, far, { x: 2, y: far.y })
    const total = before.width * 2
    expect((await paneBox(page, 'p0')).width).toBeCloseTo(total * 0.2, -1)

    // The stacked boundary: the lower tab row's empty space is the grab area too.
    const turn = page.getByTestId('session-split-divider-s1')
    await turn.hover()
    await page.getByTestId('session-split-turn-s1').click()
    await page.getByTestId('session-split-divider-s1').dblclick()
    const top = await paneBox(page, 'p0')
    const row = (await page.getByTestId('session-tab-row-p1').boundingBox())!
    const grab = { x: row.x + row.width * 0.5, y: row.y + row.height / 2 }
    await dragTo(page, grab, { x: grab.x, y: grab.y + 120 })
    expect((await paneBox(page, 'p0')).height).toBeCloseTo(top.height + 120, -1)
    await page.screenshot({ path: screenshotPath(`session-tabs-resize-stacked-${browserName}.png`) })

    const stackedDivider = await center(page.getByTestId('session-split-divider-s1'))
    await dragTo(page, stackedDivider, { x: stackedDivider.x + 40, y: stackedDivider.y - 60 })
    const size = (await paneBox(page, 'p0')).height
    expect(size).toBeCloseTo(top.height + 60, -1)

    // A Terminal binds to one element for life, so the same node means the screen survived.
    expect(await terminal!.evaluate((node) => node.isConnected)).toBe(true)

    await page.reload()
    await expect(page.getByTestId('session-pane-p1')).toBeVisible()
    await dismissSizeBanner(page)
    expect((await paneBox(page, 'p0')).height).toBeCloseTo(size, -1)
  })
})
