import { expect, test, type Page } from '@playwright/test'
import { readFile, writeFile } from 'node:fs/promises'

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

test.describe.configure({ mode: 'serial' })

test('first run creates the exact starter catalog when the private file is absent', async ({ page }) => {
  await page.goto(seedServer)
  await expect(page.getByTestId('feed-item')).toHaveCount(6)
  const state = await smoke(page)
  const seeded = await readFile(state.actionsPath, 'utf8')
  expect(seeded).toBe(`version: 1
actions:
  - id: review-pr
    label: Review PR
    type: launch-session
    show_in_detail: true
    applies_to: [pr]
    repo_template: "https://github.com/{{ .Payload.repo }}.git"
    prompt_template: |
      Review pull request {{ .Payload.url }}: {{ .Payload.title }}

      {{ .Payload.body }}
  - id: start-implementation
    label: Start implementation
    type: launch-session
    show_in_detail: true
    applies_to: [issue]
    repo_template: "https://github.com/{{ .Payload.repo }}.git"
    prompt_template: |
      Implement issue {{ .Payload.url }}: {{ .Payload.title }}

      {{ .Payload.body }}
`)
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
  await page.getByTestId('action-type').selectOption('shell')
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
  await expect.poll(async () => (await smoke(page)).outputCommands.filter((command) => command.actionId === shell)).toEqual([
    expect.objectContaining({ key: 'pr2841', status: 'done', stdout: 'smoke-stdout', stderr: 'smoke-stderr', lastError: '' }),
  ])
  await page.locator(`[data-id="${shell}"]`).click()
  await expect(page.getByTestId('toast').filter({ hasText: 'already run' })).toBeVisible()
  expect((await smoke(page)).outputCommands.filter((command) => command.actionId === shell)).toHaveLength(1)

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
    result: { session: { id: expect.any(String), name: `${templated}-pr2841` } },
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
    result: { session: { id: expect.any(String), name: interactiveName } },
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
