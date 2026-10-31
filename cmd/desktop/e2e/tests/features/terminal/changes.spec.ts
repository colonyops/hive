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
const COMMITS = [
  { id: 'c2c2c2c2c2', subject: 'Second commit', author: 'Hive', unix: 1760000200 },
  { id: 'c1c1c1c1c1', subject: 'First commit', author: 'Hive', unix: 1760000100 },
]
const noState = { binary: false, truncated: false, unavailable: false, gone: false, error: '' }

async function changesFixture(page: Page, sent: unknown[][] = []) {
  const availability = ['terminalservice', 'agentsservice', 'popupterminalservice'].map((name) => bindingID(name, 'Available'))
  const list = bindingID('sessionservice', 'ListSessions')
  const scratch = bindingID('terminalservice', 'Scratch')
  const changes = bindingID('sessionservice', 'SessionChanges')
  const diffs = bindingID('sessionservice', 'SessionFileDiffs')
  const branches = bindingID('sessionservice', 'SessionBranches')
  const targets = bindingID('reviewservice', 'ChangesTargets')
  const send = bindingID('reviewservice', 'SendChanges')
  await page.route('**/wails/runtime', async (route) => {
    const args = route.request().postDataJSON()?.args
    let answer: unknown
    if (availability.includes(args?.methodID)) answer = { available: true, reason: '' }
    else if (args?.methodID === list) answer = [{ id: 'review', name: 'Review', slug: 'review', repo: '', state: 'active' }]
    else if (args?.methodID === scratch) answer = { slug: '', name: '' }
    else if (args?.methodID === changes) {
      const { base, against, commit } = args.args[1]
      answer = {
        resolved: true, branch: 'feat/review', defaultBranch: 'main', base, branchAvailable: true, truncated: false,
        against, commit: base === 'commit' ? commit || COMMITS[0].id : '', commits: COMMITS,
        files: base === 'branch' ? FILES : FILES.slice(0, 1),
      }
    } else if (args?.methodID === branches) {
      answer = ['refs/heads/main', 'refs/heads/release', 'refs/remotes/origin/release']
    } else if (args?.methodID === diffs) {
      answer = (args.args[2] as string[]).map((path) => ({
        path, text: PATCHES[path] ?? '', ...noState, binary: path.endsWith('.png'),
      }))
    } else if (args?.methodID === targets) {
      answer = [{ kind: 'session', id: 'review', pane: '%1', label: 'Review · claude', status: 'ready' }]
    } else if (args?.methodID === send) {
      sent.push(args.args)
      answer = null
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

    await page.getByTestId('changes-against').click()
    await page.getByTestId('changes-against-option-refs/heads/release').click()
    await expect(page.getByTestId('changes-against')).toContainText('release')
    await page.screenshot({ path: screenshotPath(`changes-against-${browserName}.png`) })

    await page.getByTestId('changes-target').click()
    await expect(page.getByTestId('changes-target-option-c1c1c1c1c1')).toContainText('First commit')
    await page.screenshot({ path: screenshotPath(`changes-target-${browserName}.png`) })
    await page.getByTestId('changes-target-option-uncommitted').click()
    await expect(page.getByTestId('changes-total')).toHaveText('+3-1')
    await expect(page.getByTestId('changes-file-web/ui/App.vue-header')).toHaveCount(0)

    await page.keyboard.press('[')
    await expect(page.getByTestId('changes-commit-position')).toHaveText('2/2')
    await page.keyboard.press('[')
    await expect(page.getByTestId('changes-commit-position')).toHaveText('1/2')
    await expect(page.getByTestId('changes-target')).toContainText('First commit')
    await page.screenshot({ path: screenshotPath(`changes-commit-${browserName}.png`) })
    await page.getByTestId('changes-target').click()
    await page.getByTestId('changes-target-option-branch').click()

    await page.getByTestId('changes-folders-toggle').click()
    await expect(page.getByTestId('changes-row-web/ui/App.vue')).toHaveCount(0)
    await page.getByTestId('changes-folders-toggle').click()
    await expect(page.getByTestId('changes-row-web/ui/App.vue')).toBeVisible()
    await page.keyboard.press('b')
    await expect(page.getByTestId('changes-files')).toBeHidden()
    await page.screenshot({ path: screenshotPath(`changes-no-rail-${browserName}.png`) })
    await page.keyboard.press('b')
    await expect(page.getByTestId('changes-files')).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(view).toBeHidden()
  })

  async function openChanges(page: Page) {
    await page.goto('/#/terminal/review')
    await expect(page.locator('[data-file-drop-target]:visible').last()).toBeVisible()
    await page.keyboard.press('Meta+k')
    await page.getByTestId('command-palette-input').fill('Toggle Changes')
    await page.getByTestId('command-palette-command-title').filter({ hasText: 'Toggle Changes' }).click()
    await expect(page.getByTestId('changes-stream')).toContainText('return err')
  }

  test('comments on selected lines and sends the review to the agent (mock terminal)', async ({ page, browserName }) => {
    const sent: unknown[][] = []
    await changesFixture(page, sent)
    await openChanges(page)
    const stream = page.getByTestId('changes-stream')

    const from = stream.locator('[data-column-number="11"][data-line-type="change-addition"]').first()
    const to = stream.locator('[data-column-number="12"][data-line-type="change-addition"]').first()
    await from.hover()
    await page.mouse.down()
    await to.hover()
    await page.mouse.up()

    const popover = page.getByTestId('changes-comment-popover')
    await expect(popover).toContainText('changes.go L11-12')
    await page.screenshot({ path: screenshotPath(`changes-composer-${browserName}.png`) })
    await page.getByTestId('changes-comment-text').fill('Wrap this error.')
    await page.getByTestId('changes-comment-save').click()
    await expect(popover).toBeHidden()

    const review = page.getByTestId('changes-review')
    await expect(review).toBeHidden()
    await page.getByTestId('changes-review-toggle').click()
    await expect(review).toContainText('Wrap this error.')
    await expect(stream).toContainText('Wrap this error.')
    await expect(page.getByTestId('changes-send-count')).toHaveText('1')
    await expect(page.getByTestId('changes-row-internal/platform/git/changes.go-comments')).toHaveText('1')
    await page.screenshot({ path: screenshotPath(`changes-review-${browserName}.png`) })

    await page.getByTestId('changes-send').click()
    await expect(page.getByTestId('changes-send')).toBeHidden()
    expect(sent).toHaveLength(1)
    expect(sent[0][0]).toBe('review')
    expect(sent[0][1]).toBe('%1')
    expect(sent[0][2]).toMatchObject([
      { path: 'internal/platform/git/changes.go', startLine: 11, endLine: 12, text: 'Wrap this error.' },
    ])
    await expect(stream).not.toContainText('Wrap this error.')
    await expect(review).toBeHidden()

    await page.getByTestId('changes-review-toggle').click()
    await expect(page.getByTestId('changes-sent-0')).toContainText('1 comment')
    await page.screenshot({ path: screenshotPath(`changes-sent-${browserName}.png`) })
    await page.getByTestId('changes-sent-0-restore').click()
    await expect(stream).toContainText('Wrap this error.')
    await expect(page.getByTestId('changes-send-count')).toHaveText('1')
  })

  test('comments on code selected with the mouse (mock terminal)', async ({ page }) => {
    await changesFixture(page)
    await openChanges(page)
    const line = page.getByTestId('changes-stream').locator('[data-line="11"][data-line-type="change-addition"]').first()
    await line.dblclick({ position: { x: 60, y: 10 } })
    const popover = page.getByTestId('changes-comment-popover')
    await expect(popover).toContainText('L11')
    await page.keyboard.press('Escape')
    await expect(popover).toBeHidden()
    await expect(page.getByTestId('changes-view')).toBeVisible()
  })

  test('searches every diff and steps through the matches (mock terminal)', async ({ page, browserName }) => {
    await changesFixture(page)
    await openChanges(page)

    await page.keyboard.press('/')
    const input = page.getByTestId('changes-search-input')
    await expect(input).toBeFocused()
    await input.fill('return')
    await expect(page.getByTestId('changes-search-count')).toHaveText('1 of 2')
    await page.screenshot({ path: screenshotPath(`changes-search-${browserName}.png`) })
    await input.press('Enter')
    await expect(page.getByTestId('changes-search-count')).toHaveText('2 of 2')
    await input.press('Enter')
    await expect(page.getByTestId('changes-search-count')).toHaveText('1 of 2')
    await input.fill('nothing like this')
    await expect(page.getByTestId('changes-search-count')).toHaveText('No matches')

    await input.press('Escape')
    await expect(input).toHaveValue('')
    await input.press('Escape')
    await expect(page.getByTestId('changes-search')).toBeHidden()
    await expect(page.getByTestId('changes-view')).toBeVisible()
  })

  test('the file rail and the review panel keep the width they were given (mock terminal)', async ({ page }) => {
    await changesFixture(page)
    await openChanges(page)
    await page.keyboard.press('r')
    const review = page.getByTestId('changes-review')
    await expect(review).toBeVisible()

    const handle = page.getByTestId('resize-handle-changes-review')
    const before = (await review.boundingBox())!.width
    const box = (await handle.boundingBox())!
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
    await page.mouse.down()
    await page.mouse.move(box.x - 60, box.y + box.height / 2)
    await page.mouse.up()
    await expect.poll(async () => (await review.boundingBox())!.width).toBeGreaterThan(before + 40)
    const resized = (await review.boundingBox())!.width

    await page.reload()
    await page.keyboard.press('Meta+k')
    await page.getByTestId('command-palette-input').fill('Toggle Changes')
    await page.getByTestId('command-palette-command-title').filter({ hasText: 'Toggle Changes' }).click()
    await expect(review).toBeHidden()
    await page.keyboard.press('r')
    await expect(review).toBeVisible()
    expect((await review.boundingBox())!.width).toBe(resized)
  })
})
