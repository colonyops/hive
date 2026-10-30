import type { Page } from '@playwright/test'
import { screenshotPath } from '../../../fixtures/paths.js'
import { expect, tag, test } from '../../../fixtures/test.js'
import { bindingID } from '../../../fixtures/wails.js'

// Server builds cannot run terminals, so the session list and both git reads are
// mocked. The go tests and test/remote cover the reads themselves.
const windowState = {
  windowId: '@1', name: 'Agent', active: true, activePane: '%1', width: 100, height: 30,
  layout: { paneId: '%1', x: 0, y: 0, width: 100, height: 30, cells: [] },
}
const file = (path: string, status: string, additions: number, deletions: number, extra = {}) => ({
  path, oldPath: '', status, additions, deletions, binary: false, stamp: '1', ...extra,
})
const FILES = [
  file('internal/platform/git/changes.go', 'modified', 3, 1),
  file('internal/platform/git/changes_test.go', 'untracked', 0, 0),
  file('web/ui/App.vue', 'added', 2, 0),
  file('docs/old.png', 'deleted', 0, 0, { binary: true }),
]
const PATCHES: Record<string, string> = {
  'internal/platform/git/changes.go': [
    '@@ -10,3 +10,5 @@ func (e *Executor) MergeBase() {',
    ' func read() error {',
    '-\treturn nil',
    '+\tif err := run(); err != nil {',
    '+\t\treturn err',
    '+\t}',
    ' }',
    '@@ -40 +42 @@',
    '-var old = 1',
    '+var new = 2',
    '',
  ].join('\n'),
  'internal/platform/git/changes_test.go': '@@ -0,0 +1,2 @@\n+package git_test\n+\n',
  'web/ui/App.vue': '@@ -0,0 +1,2 @@\n+<template>\n+</template>\n',
}
const noState = { binary: false, truncated: false, unavailable: false, gone: false, error: '' }

async function changesFixture(page: Page) {
  const availability = ['terminalservice', 'agentsservice', 'popupterminalservice'].map((name) => bindingID(name, 'Available'))
  const list = bindingID('sessionservice', 'ListSessions')
  const scratch = bindingID('terminalservice', 'Scratch')
  const changes = bindingID('sessionservice', 'SessionChanges')
  const diffs = bindingID('sessionservice', 'SessionFileDiffs')
  await page.route('**/wails/runtime', async (route) => {
    const args = route.request().postDataJSON()?.args
    let answer: unknown
    if (availability.includes(args?.methodID)) answer = { available: true, reason: '' }
    else if (args?.methodID === list) answer = [{ id: 'review', name: 'Review', slug: 'review', repo: '', state: 'active' }]
    else if (args?.methodID === scratch) answer = { slug: '', name: '' }
    else if (args?.methodID === changes) {
      const base = args.args[1]
      answer = {
        resolved: true, branch: 'feat/review', defaultBranch: 'main', base, branchAvailable: true, truncated: false,
        files: base === 'branch' ? FILES : FILES.slice(0, 1),
      }
    } else if (args?.methodID === diffs) {
      answer = (args.args[2] as string[]).map((path) => ({
        path, text: PATCHES[path] ?? '', ...noState, binary: path.endsWith('.png'),
      }))
    } else return route.fallback()
    await route.fulfill({ json: answer })
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
    if (path === 'attach') answer = { windows: [windowState] }
    else if (path === 'windows/list') answer = { sessions: { review: [windowState] } }
    else if (path === 'agents/sessions' || path === 'agents/sessions/all') answer = { sessions: [] }
    else if (path === 'agents/sessions/activity') answer = { items: [] }
    else if (path === 'agents/workspaces') answer = { root: '/w', available: true, workspaces: [], presets: [], editor: { command: '', title: '' } }
    else return route.fulfill({ status: 204, headers })
    await route.fulfill({ json: answer, headers })
  })
  await page.routeWebSocket('**/api/terminal/stream?*', (socket) => {
    socket.send(Buffer.concat([Buffer.from([0, 2]), Buffer.from('@1'), Buffer.from([2]), Buffer.from('%1agent> ')]))
  })
}

test.describe('session changes', { tag: [tag.terminal] }, () => {
  test('reviews every diff in one scroll from the keyboard (mock terminal)', async ({ page, browserName }) => {
    await changesFixture(page)
    await page.goto('/#/terminal/review')
    await expect(page.locator('[data-file-drop-target]:visible').last()).toBeVisible()

    await page.keyboard.press('Meta+k')
    await page.getByTestId('command-palette-input').fill('Toggle Changes')
    await page.getByTestId('command-palette-command-title').filter({ hasText: 'Toggle Changes' }).click()
    const view = page.getByTestId('changes-view')
    await expect(view).toBeVisible()
    await expect(page.getByTestId('changes-branch')).toHaveText('feat/review')
    await expect(page.getByTestId('changes-total')).toHaveText('+5-1')

    const stream = page.getByTestId('changes-stream')
    await expect(stream).toContainText('return err')
    await expect(stream).toContainText('package git_test')
    await expect(stream).toContainText('Binary file')
    await page.screenshot({ path: screenshotPath(`changes-unified-${browserName}.png`) })

    await page.keyboard.press('v')
    await expect(page.getByTestId('changes-progress')).toHaveText('1/4 viewed')

    await page.keyboard.press('n')
    await expect(stream).toContainText('var new = 2')
    await page.keyboard.press('j')
    await expect(page.getByTestId('changes-file-web/ui/App.vue-header')).toBeVisible()

    await page.keyboard.press('s')
    await expect(stream.locator('[data-diff-type="split"]').first()).toBeVisible()
    await page.screenshot({ path: screenshotPath(`changes-split-${browserName}.png`) })
    await page.keyboard.press('s')

    await page.getByTestId('changes-base-uncommitted').click()
    await expect(page.getByTestId('changes-total')).toHaveText('+3-1')
    await expect(page.getByTestId('changes-file-web/ui/App.vue-header')).toHaveCount(0)

    await page.keyboard.press('Escape')
    await expect(view).toBeHidden()
  })
})
