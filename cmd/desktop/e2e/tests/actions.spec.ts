import { type Page } from '@playwright/test'
import { readFile, writeFile } from 'node:fs/promises'
import { expect, resetServerState, test } from './fixtures.js'

type SmokeState = {
  runId: string
  actionsPath: string
  sessions: Array<{ id: string; name: string; remote: string }>
  messages: Array<{ id: string; topic: string; payload: string; sender: string; sessionId: string }>
  outputCommands: Array<{ id: number; actionId: string; key: string; status: string; lastError: string; stdout: string; stderr: string; result: unknown }>
}

const actionServer = 'http://127.0.0.1:8936'
const seedServer = 'http://127.0.0.1:8937'

async function smoke(page: Page): Promise<SmokeState> {
  const response = await page.request.get(new URL('/_e2e/actions', page.url()).toString())
  expect(response.ok()).toBeTruthy()
  return await response.json() as SmokeState
}

async function select(page: Page, id: string): Promise<void> {
  await page.locator(`[data-testid="feed-item"][data-id="${id}"]`).click()
}

function action(runID: string, suffix: string): string { return `smoke-${runID}-${suffix}` }

const nonEmpty = expect.stringMatching(/.+/)

// The three surfaces the catalog's order has to agree on: the settings list,
// the file itself, and the detail pane's action cards.
async function rowIds(page: Page): Promise<string[]> {
  return page.locator('[data-testid^="action-row-"]').evaluateAll((rows) => rows.map((row) => (row as HTMLElement).dataset.testid!.slice('action-row-'.length)))
}
function fileIds(yaml: string): string[] {
  return [...yaml.matchAll(/- id: (\S+)/g)].map((match) => match[1])
}
async function cardIds(page: Page): Promise<string[]> {
  return page.getByTestId('action-card').evaluateAll((cards) => cards.map((card) => (card as HTMLElement).dataset.id!))
}

// Serial: later tests read durable rows earlier tests create, so this file is
// one ordered journey. It opts out of the per-test reset (which would sever
// that chain) in favor of one reset per file execution: a serial-group retry
// re-runs beforeAll in its fresh worker, so every attempt starts from the
// server baseline and the exact durable-row counts below (output_commands,
// messages) hold on retries too — the interim retries: 0 override this file
// carried is therefore gone.
test.use({ serverStateReset: 'per-file' })
test.describe.configure({ mode: 'serial' })
test.beforeAll(async ({}, testInfo) => {
  // The action-seed server (seedServer) is only observed read-only by this
  // file, so the action-smoke project server is the one needing a baseline.
  await resetServerState(testInfo.project.use.baseURL)
})

// The starter catalog's exact bytes are pinned in Go, by
// actions.TestSeedDefaultsIfMissingInstallsExactBytesOnce. Restating them here
// only produced a second copy that rotted the next time the catalog grew, so
// this asserts what a real first run is the only thing that can show: the app
// wrote that catalog to the configured path.
test('first run seeds the starter catalog when the private file is absent', async ({ page }) => {
  await page.goto(seedServer)
  await expect(page.getByTestId('feed-item')).toHaveCount(6)
  const state = await smoke(page)
  const seeded = await readFile(state.actionsPath, 'utf8')

  expect(seeded).toMatch(/^version: 1$/m)
  expect(seeded).toMatch(/^actions:$/m)
  expect(seeded).toMatch(/^launchers:$/m)
  for (const id of ['review-pr', 'address-review-feedback', 'start-implementation', 'copy-checkout', 'open-on-github', 'share-to-team']) {
    expect(seeded).toContain(`- id: ${id}`)
  }
})

test.beforeEach(async ({ page }) => {
  await page.goto(actionServer)
  await expect(page.getByTestId('feed-item')).toHaveCount(6)
})

test('scopes shown-in-detail actions to PR and issue items', async ({ page }) => {
  const state = await smoke(page)
  await select(page, 'pr2841')
  await expect(page.getByTestId('action-card')).toHaveCount(4)
  await expect(page.locator(`[data-id="${action(state.runId, 'hidden')}"]`)).toHaveCount(0)
  await select(page, 'iss1190')
  await expect(page.getByTestId('action-card')).toHaveCount(3)
})

test('creates, edits, and deletes through the slideover and common confirmation dialog', async ({ page }) => {
  await page.getByTestId('application-settings').click()
  await page.getByTestId('settings-category-actions').click()
  await expect(page.getByTestId('actions-settings')).toBeVisible()
  await page.getByTestId('action-create').click()
  await page.getByTestId('action-id').fill('smoke-created')
  await page.getByTestId('action-label').fill('Created smoke action')
  await page.getByTestId('action-type').click()
  await page.getByTestId('action-type-option-shell').click()
  await page.getByTestId('action-shell-command').fill('/usr/bin/true')
  await page.getByTestId('action-show-in-detail').click()
  await page.getByTestId('action-save').click()
  await expect(page.getByTestId('action-row-smoke-created')).toContainText('Flow-only')

  await page.getByTestId('action-row-smoke-created').getByText('Edit').click()
  await page.getByTestId('action-label').fill('Edited smoke action')
  await page.getByTestId('action-save').click()
  await expect(page.getByTestId('action-row-smoke-created')).toContainText('Edited smoke action')

  await page.getByTestId('action-row-smoke-created').getByRole('button', { name: 'Delete' }).click()
  await expect(page.getByRole('alertdialog')).toContainText('Delete action')
  await page.getByRole('button', { name: 'Delete action' }).click()
  await expect(page.getByTestId('action-row-smoke-created')).toHaveCount(0)
})

test('drag-reorders the catalog and honors that order in settings, the detail pane, and on disk', async ({ page }) => {
  const state = await smoke(page)
  const original = await readFile(state.actionsPath, 'utf8')
  const moved = action(state.runId, 'failed-shell')
  const first = action(state.runId, 'pr')
  const inDetail = [first, action(state.runId, 'message'), action(state.runId, 'template-launch')]
  await page.getByTestId('application-settings').click()
  await page.getByTestId('settings-category-actions').click()
  // rowIds snapshots through evaluateAll, which does not auto-wait, so one
  // visible row does not mean the list finished rendering — reading it here
  // could return a short list, or an empty one. Poll for the head before
  // taking the snapshot the rest of the test is built on.
  await expect(page.getByTestId(`action-row-${moved}`)).toBeVisible()
  await expect.poll(async () => (await rowIds(page))[0]).toBe(first)
  const before = await rowIds(page)

  // Drop on the top edge of the first row: the catalog's new head.
  await page.getByTestId(`action-row-${moved}`).dragTo(page.getByTestId(`action-row-${first}`), { targetPosition: { x: 60, y: 3 } })
  const reordered = [moved, ...before.filter((id) => id !== moved)]
  await expect.poll(() => rowIds(page)).toEqual(reordered)
  await expect.poll(async () => fileIds(await readFile(state.actionsPath, 'utf8'))).toEqual(reordered)

  // The detail pane renders that order, still filtered to applicable actions.
  await page.goto(actionServer)
  await select(page, 'pr2841')
  await expect.poll(() => cardIds(page)).toEqual([moved, ...inDetail])

  // A hand edit to the file's order is picked up by the watcher and wins.
  await writeFile(state.actionsPath, original, 'utf8')
  await expect.poll(() => cardIds(page)).toEqual([...inDetail, moved])
})

test('external malformed actions keep last-good catalog and recover after repair', async ({ page }) => {
  const state = await smoke(page)
  const original = await readFile(state.actionsPath, 'utf8')
  await page.getByTestId('application-settings').click()
  await page.getByTestId('settings-category-actions').click()
  await expect(page.getByTestId('action-row-' + action(state.runId, 'pr'))).toBeVisible()

  await writeFile(state.actionsPath, 'version: 1\nactions:\n  - id: broken\n    type: nope\n', 'utf8')
  await expect(page.getByTestId('actions-error')).toBeVisible()
  await expect(page.getByTestId('action-row-' + action(state.runId, 'pr'))).toBeVisible()

  await writeFile(state.actionsPath, original, 'utf8')
  await expect(page.getByTestId('actions-error')).toHaveCount(0)
  await expect(page.getByTestId('action-row-' + action(state.runId, 'pr'))).toBeVisible()
})

test('persists shell output, failure diagnostics, and durable duplicate rejection', async ({ page }) => {
  const state = await smoke(page)
  await select(page, 'pr2841')
  const shell = action(state.runId, 'pr')
  await page.locator(`[data-id="${shell}"]`).click()
  await expect(page.getByTestId('toast')).toContainText('Smoke PR completed')
  const jobsChip = page.getByTestId('titlebar-jobs')
  await expect(jobsChip).toBeVisible()
  await jobsChip.click()
  await expect(page.getByTestId('jobs-popover')).toContainText('Smoke PR')
  await expect(page.getByTestId('jobs-popover')).toContainText('Completed')
  await expect(jobsChip).toBeHidden({ timeout: 7_000 })
  await expect.poll(async () => (await smoke(page)).outputCommands.filter((command) => command.actionId === shell)).toEqual([
    expect.objectContaining({ key: 'pr2841', status: 'done', stdout: 'smoke-stdout', stderr: 'smoke-stderr', lastError: '' }),
  ])
  await page.locator(`[data-id="${shell}"]`).click()
  await expect(page.getByTestId('action-rerun-confirmation')).toContainText('has already run for this item')
  await page.getByTestId('action-rerun-confirmation-confirm').click()
  await expect(page.getByTestId('toast')).toContainText('Smoke PR completed')
  await expect.poll(async () => (await smoke(page)).outputCommands.filter((command) => command.actionId === shell)).toHaveLength(2)

  const failing = action(state.runId, 'failed-shell')
  await page.locator(`[data-id="${failing}"]`).click()
  await expect(page.getByTestId('action-failure')).toContainText('shell: command failed')
  await expect(page.getByTestId('action-stdout')).toContainText('failing-stdout')
  await expect(page.getByTestId('action-stderr')).toContainText('failing-stderr')
  await expect.poll(async () => (await smoke(page)).outputCommands.find((command) => command.actionId === failing)).toEqual(expect.objectContaining({ status: 'failed', stdout: 'failing-stdout', stderr: 'failing-stderr', lastError: expect.stringContaining('shell: command failed'), result: null }))
  await page.reload()
  await select(page, 'pr2841')
  await expect(page.getByTestId('action-failure')).toContainText('shell: command failed')
})

test('publishes rendered message and launches templated and dialog sessions against the local fixture', async ({ page }) => {
  const state = await smoke(page)
  await select(page, 'pr2841')
  await page.locator(`[data-id="${action(state.runId, 'message')}"]`).click()
  await expect(page.getByTestId('toast')).toContainText(`Published message to smoke.${state.runId} as hive-desktop`)
  await expect.poll(async () => (await smoke(page)).messages).toEqual([
    expect.objectContaining({ topic: `smoke.${state.runId}`, payload: 'message for pr2841', sender: 'hive-desktop', sessionId: '' }),
  ])
  await expect.poll(async () => (await smoke(page)).outputCommands.find((command) => command.actionId === action(state.runId, 'message'))).toEqual(expect.objectContaining({
    key: 'pr2841',
    status: 'done',
    result: { message: { topic: `smoke.${state.runId}`, sender: 'hive-desktop' } },
  }))

  const templated = action(state.runId, 'template-launch')
  await page.locator(`[data-id="${templated}"]`).click()
  await expect(page.getByTestId('toast').filter({ hasText: 'Created session' })).toBeVisible()
  await expect.poll(async () => (await smoke(page)).sessions.find((session) => session.name === `${templated}-pr2841`)).toEqual(expect.objectContaining({ remote: expect.stringContaining('remote.git') }))
  await expect.poll(async () => (await smoke(page)).outputCommands.find((command) => command.actionId === templated)).toEqual(expect.objectContaining({
    key: 'pr2841',
    status: 'done',
    result: { session: { id: expect.any(String), name: `${templated}-pr2841`, slug: nonEmpty, path: nonEmpty } },
    stdout: `post-hook pr2841 ${templated}-pr2841`,
  }))

  await select(page, 'iss1190')
  await page.locator(`[data-id="${action(state.runId, 'dialog-launch')}"]`).click()
  await expect(page.getByTestId('create-session-dialog')).toBeVisible()
  const interactiveName = `smoke-${state.runId}-interactive`
  await page.getByTestId('session-name').fill(interactiveName)
  await page.getByTestId('create-session-submit').click()
  await expect(page.getByTestId('toast').filter({ hasText: `Created session ${interactiveName}` })).toBeVisible()
  await expect.poll(async () => (await smoke(page)).sessions.find((session) => session.name === interactiveName)).toEqual(expect.objectContaining({ remote: expect.stringContaining('remote.git') }))
  await expect.poll(async () => (await smoke(page)).outputCommands.find((command) => command.actionId === action(state.runId, 'dialog-launch'))).toEqual(expect.objectContaining({
    key: 'iss1190',
    status: 'done',
    result: { session: { id: expect.any(String), name: interactiveName, slug: nonEmpty, path: nonEmpty } },
  }))
})

test('failed launch has no session identity and keeps durable diagnostics', async ({ page }) => {
  const state = await smoke(page)
  await select(page, 'iss1190')
  const failed = action(state.runId, 'failed-launch')
  await page.locator(`[data-id="${failed}"]`).click()
  await expect(page.getByTestId('action-failure')).toContainText('create hive session')
  await expect.poll(async () => (await smoke(page)).outputCommands.find((command) => command.actionId === failed)).toEqual(expect.objectContaining({ status: 'failed', lastError: expect.stringContaining('create hive session'), result: null }))
  expect((await smoke(page)).sessions.some((session) => session.name === `${failed}-iss1190`)).toBeFalsy()
})
