import { expect, test } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const expectedItems = [
  ['pr2841', 'PR'],
  ['iss1190', 'Issue'],
  ['pr2838', 'PR'],
  ['iss1204', 'Issue'],
  ['pr2830', 'PR'],
  ['iss1177', 'Issue'],
] as const

test.beforeEach(async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('feed-item')).toHaveCount(6)
})

test('renders the mock feed with pr2841 selected by default', async ({ page }) => {
  const feedItems = page.getByTestId('feed-item')
  expect(await feedItems.evaluateAll((items) => items.map((item) => item.getAttribute('data-id')))).toEqual(
    expectedItems.map(([id]) => id),
  )
  expect(await feedItems.locator('[data-testid="kind-badge"]').evaluateAll((badges) => badges.map((badge) => badge.getAttribute('data-kind')))).toEqual(
    expectedItems.map(([, kind]) => kind),
  )
  await expect(page.getByTestId('detail-pane')).toContainText('batch_spawn: fix detached tmux env & PATH propagation')
  await expect(page.getByTestId('detail-pane')).toContainText('hive/core #2841')
  await expect(page.getByTestId('detail-pane')).toContainText('fix/2841-batch-spawn-env')
})

test('updates the detail pane and actions for PRs and issues', async ({ page }) => {
  await page.locator('[data-testid="feed-item"][data-id="pr2838"]').click()
  await expect(page.getByTestId('detail-pane')).toContainText('OAuth device flow for in-app GitHub auth')
  await expect(page.getByTestId('detail-pane')).toContainText('hive/desktop #2838')
  await expect(page.getByTestId('detail-pane')).toContainText('feat/2838-oauth-device-flow')
  await expect(page.getByTestId('action-card')).toHaveCount(4)
  await expect(page.getByTestId('primary-action')).toHaveText('Run')
  await expect(page.getByTestId('action-card').first()).toContainText('Review PR')

  await page.locator('[data-testid="feed-item"][data-id="iss1190"]').click()
  await expect(page.getByTestId('detail-pane')).toContainText('Feed source: mirror GitHub notifications inbox')
  await expect(page.getByTestId('detail-pane')).toContainText('hive/desktop #1190')
  await expect(page.getByTestId('detail-pane')).toContainText('feat/1190-notifications-feed')
  await expect(page.getByTestId('action-card')).toHaveCount(4)
  await expect(page.getByTestId('primary-action')).toHaveText('Run')
  await expect(page.getByTestId('action-card').first()).toContainText('Start implementation')
})

test('filters the feed to its three unread items', async ({ page }) => {
  await page.getByTestId('unread-chip').click()
  const unreadItems = page.getByTestId('feed-item')
  await expect(unreadItems).toHaveCount(3)
  expect(await unreadItems.evaluateAll((items) => items.map((item) => item.getAttribute('data-id')))).toEqual([
    'pr2841', 'iss1190', 'iss1204',
  ])
})

test('shows the single profile in the rail and sidebar', async ({ page }) => {
  await expect(page.getByTestId('profile-tile')).toHaveCount(1)
  await expect(page.getByTestId('breadcrumb-profile-name')).toHaveText('Frontend Triage')
  await expect(page.getByTestId('sidebar-profile-name')).toHaveText('Frontend Triage')
})

test('shows an inert toast for actions without changing the selection', async ({ page }) => {
  const detail = page.getByTestId('detail-pane')
  await expect(detail).toContainText('hive/core #2841')
  await page.getByTestId('action-card').first().click()
  await expect(page.getByTestId('toast')).toHaveText('Not wired up yet')
  await expect(detail).toContainText('hive/core #2841')
})

test('opens, filters, runs, and dismisses the command palette', async ({ page }) => {
  await page.keyboard.press('Meta+k')
  const palette = page.getByTestId('command-palette')
  await expect(palette).toBeVisible()
  const input = page.getByTestId('command-palette-input')
  await input.fill('notifications')
  // The feed matches twice now: its Select entry and its feed-editor Edit entry.
  const commands = page.getByTestId('command-palette-command')
  await expect(commands).toHaveCount(2)
  await commands.filter({ hasText: 'Select feed: Notifications inbox' }).click()
  await expect(palette).toBeHidden()
  await expect(page.getByTestId('feed-title')).toHaveText('Notifications inbox')

  await page.keyboard.press('Meta+k')
  await expect(palette).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(palette).toBeHidden()

  await page.keyboard.press('Control+k')
  await expect(palette).toBeVisible()
})

test('captures a full-window screenshot', async ({ page }, testInfo) => {
  const screenshots = join(here, '..', 'screenshots')
  await mkdir(screenshots, { recursive: true })
  await page.screenshot({ path: join(screenshots, `full-window-${testInfo.project.name}.png`), fullPage: true })
})
