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
  await expect(page.getByTestId('session-tab-terminal')).toBeVisible()
  await expect(page.getByTestId('terminal-window-row').first()).toBeVisible()
}

async function addTab(page: Page, pane: number, label: string) {
  await page.getByTestId(`session-tab-add-${pane}`).click()
  await page.getByTestId(`session-tab-menu-${pane}`).getByText(label, { exact: true }).click()
}

function tabIds(page: Page, pane: number): Promise<string[]> {
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

const paneBox = async (page: Page, pane: number) => (await page.getByTestId(`session-pane-${pane}`).boundingBox())!

test.describe('session tabs', { tag: [tag.terminal] }, () => {
  test('opens each tab once, and restores a session after another', async ({ page, browserName }) => {
    await fixture(page)
    await addTab(page, 0, 'Docs')
    await addTab(page, 0, 'Diff')
    await addTab(page, 0, 'Tasks')
    await addTab(page, 0, 'Diff')
    expect(await tabIds(page, 0)).toEqual(['terminal', 'docs', 'diff', 'tasks'])
    await expect(page.getByTestId('session-tab-diff')).toHaveAttribute('data-shown', 'true')
    await expect(page.getByTestId('changes-view')).toBeVisible()
    await expect(page.getByTestId('session-tab-close-terminal')).toHaveCount(0)
    await expect(page.getByTestId('titlebar-toggle-preview')).toBeDisabled()
    await page.screenshot({ path: screenshotPath(`session-tabs-tabs-${browserName}.png`) })

    await page.locator('[data-testid="terminal-session-row"][data-slug="other"]').click()
    await expect(page.getByTestId('session-tab-terminal')).toHaveAttribute('data-shown', 'true')
    expect(await tabIds(page, 0)).toEqual(['terminal'])

    await page.locator('[data-testid="terminal-session-row"][data-slug="review"]').click()
    await expect(page.getByTestId('session-tab-diff')).toHaveAttribute('data-shown', 'true')
    expect(await tabIds(page, 0)).toEqual(['terminal', 'docs', 'diff', 'tasks'])
  })

  test('opens Diff from the counts in the status bar', async ({ page }) => {
    await fixture(page)
    await page.getByTestId('session-status-diff').click()
    await expect(page.getByTestId('changes-view')).toBeVisible()
    await expect(page.getByTestId('session-status-host')).toHaveText('Local')
  })

  test('splits through the picker, and puts Terminal beside Diff', async ({ page, browserName }) => {
    await fixture(page)
    await page.getByTestId('session-split-row').click()
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
    await expect(page.getByTestId('session-pane-1')).toHaveCount(0)
  })

  test('shows a second window beside the first, and hiding it leaves it in the tree', async ({ page }) => {
    await fixture(page)
    await page.getByTestId('session-split-row').click()
    await page.getByTestId('session-pane-picker-window-@2').click()
    await expect(page.locator('[data-testid="terminal-pane"][data-window-id="@1"]')).toBeVisible()
    await expect(page.locator('[data-testid="terminal-pane"][data-window-id="@2"]')).toBeVisible()

    await page.getByTestId('session-tab-close-window:@2').click()
    await expect(page.locator('[data-testid="terminal-pane"][data-window-id="@2"]')).toBeHidden()
    await expect(page.locator('[data-testid="terminal-window-row"][data-window-id="@2"]')).toBeVisible()
  })

  test('drags tabs along a row and between panes', async ({ page, browserName }) => {
    await fixture(page)
    await addTab(page, 0, 'Docs')
    await addTab(page, 0, 'Diff')
    await addTab(page, 0, 'Tasks')

    await dragTabBefore(page, 'tasks', 'docs')
    await expect.poll(() => tabIds(page, 0)).toEqual(['terminal', 'tasks', 'docs', 'diff'])

    // The Terminal tab is pinned: a drop ahead of it lands after it, and it does not drag.
    await dragTabBefore(page, 'diff', 'terminal')
    await expect.poll(() => tabIds(page, 0)).toEqual(['terminal', 'diff', 'tasks', 'docs'])
    await dragTabBefore(page, 'terminal', 'docs')
    await expect.poll(() => tabIds(page, 0)).toEqual(['terminal', 'diff', 'tasks', 'docs'])

    await page.getByTestId('session-split-row').click()
    await page.getByTestId('session-pane-picker-window-@2').click()
    const row = (await page.getByTestId('session-tab-row-1').boundingBox())!
    await dragTo(page, await center(page.getByTestId('session-tab-diff')), { x: row.x + row.width / 2, y: row.y + row.height / 2 })
    await expect.poll(() => tabIds(page, 1)).toEqual(['window:@2', 'diff'])
    expect(await tabIds(page, 0)).toEqual(['terminal', 'tasks', 'docs'])
    await expect(page.getByTestId('changes-view')).toBeVisible()
    await page.screenshot({ path: screenshotPath(`session-tabs-drag-${browserName}.png`) })

    // Over a terminal rather than a row, which would swallow the pointer without capture.
    await page.getByTestId('session-tab-terminal').click()
    const terminal = await center(page.locator('[data-testid="terminal-pane"]:visible').first())
    await dragTo(page, await center(page.getByTestId('session-tab-window:@2')), terminal)
    await expect.poll(() => tabIds(page, 0)).toContain('window:@2')

    await page.reload()
    await expect(page.getByTestId('session-tab-terminal')).toBeVisible()
    expect(await tabIds(page, 1)).toEqual(['diff'])
  })

  test('lists Docs, Diff and Tasks in the tree', async ({ page, browserName }) => {
    await fixture(page)
    const rows = page.getByTestId('terminal-view-row')
    await expect(rows).toHaveCount(0)

    await addTab(page, 0, 'Docs')
    await addTab(page, 0, 'Diff')
    await expect(rows).toHaveCount(2)
    await expect(page.locator('[data-testid="terminal-view-row"][data-view-kind="docs"]')).toContainText('Docs')
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

  test('splits either way at any width, and keeps the pick per session', async ({ page, browserName }) => {
    await fixture(page)
    await page.getByTestId('session-split-column').click()
    await page.getByTestId('session-pane-picker-view-tasks').click()
    let [first, second] = [await paneBox(page, 0), await paneBox(page, 1)]
    expect(second.y).toBeGreaterThan(first.y + first.height - 2)
    expect(second.x).toBe(first.x)
    await page.screenshot({ path: screenshotPath(`session-tabs-stacked-${browserName}.png`) })

    await page.getByTestId('session-split-row').click()
    ;[first, second] = [await paneBox(page, 0), await paneBox(page, 1)]
    expect(second.x).toBeGreaterThan(first.x + first.width - 2)
    expect(second.y).toBe(first.y)
    await page.screenshot({ path: screenshotPath(`session-tabs-side-by-side-${browserName}.png`) })

    await page.getByTestId('session-split-column').click()
    await page.reload()
    await expect(page.getByTestId('session-pane-1')).toBeVisible()
    ;[first, second] = [await paneBox(page, 0), await paneBox(page, 1)]
    expect(second.y).toBeGreaterThan(first.y)

    // Narrow enough that a split nobody picked a direction for stacks, a pick still holds.
    await page.getByTestId('session-split-row').click()
    await page.setViewportSize({ width: 760, height: 700 })
    ;[first, second] = [await paneBox(page, 0), await paneBox(page, 1)]
    expect(second.x).toBeGreaterThan(first.x)

    await page.locator('[data-testid="terminal-session-row"][data-slug="other"]').click()
    await page.getByTestId('session-split-row').click()
    await page.getByTestId('session-split-row').click()
    await expect(page.getByTestId('session-pane-1')).toHaveCount(0)
  })

  test('drags the boundary to resize both ways, keeps it, and clamps it', async ({ page, browserName }) => {
    await fixture(page)
    await page.getByTestId('session-split-row').click()
    await page.getByTestId('session-pane-picker-window-@2').click()
    const terminal = await page.locator('[data-testid="terminal-pane"][data-window-id="@1"]').elementHandle()

    const before = await paneBox(page, 0)
    const divider = await center(page.getByTestId('session-split-divider'))
    await dragTo(page, divider, { x: divider.x + 180, y: divider.y })
    expect((await paneBox(page, 0)).width).toBeCloseTo(before.width + 180, -1)
    await page.screenshot({ path: screenshotPath(`session-tabs-resize-${browserName}.png`) })

    const far = await center(page.getByTestId('session-split-divider'))
    await dragTo(page, far, { x: 2, y: far.y })
    const total = before.width * 2
    expect((await paneBox(page, 0)).width).toBeCloseTo(total * 0.2, -1)

    // The stacked boundary: the second tab row's empty space is the grab area too.
    await page.getByTestId('session-split-column').click()
    const top = await paneBox(page, 0)
    const row = (await page.getByTestId('session-tab-row-1').boundingBox())!
    const grab = { x: row.x + row.width * 0.5, y: row.y + row.height / 2 }
    await dragTo(page, grab, { x: grab.x, y: grab.y + 120 })
    expect((await paneBox(page, 0)).height).toBeCloseTo(top.height + 120, -1)
    await page.screenshot({ path: screenshotPath(`session-tabs-resize-stacked-${browserName}.png`) })

    const stackedDivider = await center(page.getByTestId('session-split-divider'))
    await dragTo(page, stackedDivider, { x: stackedDivider.x, y: stackedDivider.y - 60 })
    const size = (await paneBox(page, 0)).height
    expect(size).toBeCloseTo(top.height + 60, -1)

    // A Terminal binds to one element for life, so the same node means the screen survived.
    expect(await terminal!.evaluate((node) => node.isConnected)).toBe(true)

    await page.reload()
    await expect(page.getByTestId('session-pane-1')).toBeVisible()
    expect((await paneBox(page, 0)).height).toBeCloseTo(size, -1)
  })
})
