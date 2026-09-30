import { mkdir } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from './fixtures.js'

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
  expect(await feedItems.locator('[data-testid="type-pill"]').evaluateAll((badges) => badges.map((badge) => badge.getAttribute('data-kind')))).toEqual(
    expectedItems.map(([, kind]) => kind),
  )
  await expect(page.getByTestId('detail-pane')).toContainText('batch_spawn: fix detached tmux env & PATH propagation')
  await expect(page.getByTestId('detail-pane')).toContainText('hive/core #2841')
})

// The fixture spans 45 minutes to 23 days old, so the list always breaks into
// several tiers — but which ones depends on the wall clock (a run just after
// local midnight pushes "45 minutes ago" into Yesterday), and a leading Today
// renders no separator at all. Assert the invariants that hold at any hour:
// known labels, no repeats, newest tier first.
test('separates the feed into date tiers', async ({ page }) => {
  const tiers = ['Today', 'Yesterday', 'This week', 'Last week', 'This month', 'Last month', 'Older']
  const labels = await page.getByTestId('feed-date-label').evaluateAll((els) => els.map((el) => el.textContent?.trim() ?? ''))
  expect(labels.length).toBeGreaterThan(1)
  expect(labels.filter((label) => tiers.includes(label))).toEqual(labels)
  expect(new Set(labels).size).toBe(labels.length)
  expect(labels).toEqual([...labels].sort((a, b) => tiers.indexOf(a) - tiers.indexOf(b)))
})

test('updates the detail pane and actions for PRs and issues', async ({ page }) => {
  await page.locator('[data-testid="feed-item"][data-id="pr2838"]').click()
  await expect(page.getByTestId('detail-pane')).toContainText('OAuth device flow for in-app GitHub auth')
  await expect(page.getByTestId('detail-pane')).toContainText('hive/desktop #2838')
  await expect(page.getByTestId('action-card')).toHaveCount(1)
  await expect(page.getByTestId('action-card').first()).toContainText('Review PR')

  await page.locator('[data-testid="feed-item"][data-id="iss1190"]').click()
  await expect(page.getByTestId('detail-pane')).toContainText('Feed source: mirror GitHub notifications inbox')
  await expect(page.getByTestId('detail-pane')).toContainText('hive/desktop #1190')
  await expect(page.getByTestId('action-card')).toHaveCount(1)
  await expect(page.getByTestId('action-card').first()).toContainText('Start implementation')
})

test('filters the feed to its remaining unread items', async ({ page }) => {
  // The per-test reset restores the seeded read state (pr2841, iss1190, and
  // iss1204 unread), so this test owns its precondition instead of relying on
  // an earlier test's click: read iss1190 first so the filter narrows a mixed
  // read/unread feed rather than echoing the seed.
  const readItem = page.locator('[data-testid="feed-item"][data-id="iss1190"]')
  await readItem.click()
  await expect(readItem.getByTestId('item-title')).toHaveClass(/font-normal/)

  await page.getByTestId('filter-unread').click()
  const unreadItems = page.getByTestId('feed-item')
  await expect(unreadItems).toHaveCount(2)
  expect(await unreadItems.evaluateAll((items) => items.map((item) => item.getAttribute('data-id')))).toEqual([
    'pr2841', 'iss1204',
  ])
})

test('filters by author together with search and clears back to the full feed', async ({ page }, testInfo) => {
  await page.getByTestId('view-menu-toggle').click()
  await page.getByTestId('view-author-filter').click()
  await page.getByTestId('view-author-search').fill('octo')
  await page.getByTestId('view-author-option').filter({ hasText: /^octocat$/ }).click()
  await expect(page.getByTestId('feed-item')).toHaveCount(2)
  expect(await page.getByTestId('feed-item').evaluateAll((items) => items.map((item) => item.getAttribute('data-id')))).toEqual(['iss1190', 'iss1177'])
  await expect(page.getByTestId('clear-author-filter')).toContainText('octocat')
  await page.getByTestId('feed-search').fill('notifications')
  await expect(page.getByTestId('feed-item')).toHaveCount(1)
  await page.getByTestId('feed-search').fill('')
  await page.getByTestId('view-menu-toggle').click()
  await page.getByTestId('view-author-filter').click()
  await expect(page.getByTestId('view-author-option').filter({ hasText: /^octocat$/ })).toHaveAttribute('aria-checked', 'true')
  const screenshots = join(here, '..', 'screenshots')
  await mkdir(screenshots, { recursive: true })
  await page.screenshot({ path: join(screenshots, `author-filter-${testInfo.project.name}.png`), fullPage: true })
  await page.keyboard.press('Escape')
  await page.getByTestId('clear-author-filter').click()
  await expect(page.getByTestId('feed-item')).toHaveCount(6)
})

test('archives, restores, and marks the selected inbox item unread from keyboard commands', async ({ page }) => {
  const item = page.locator('[data-testid="feed-item"][data-id="pr2841"]')
  await expect(item).toBeVisible()
  // Select explicitly; keyboard triage always applies to the detail selection,
  // not merely the first rendered row. Wait for selecting an unread item to
  // finish its read mutation before exercising its next revision-guarded write.
  await item.click()
  await expect(item.getByTestId('item-title')).toHaveClass(/font-normal/)
  await page.keyboard.press('Shift+U')
  await expect(item.getByTestId('item-title')).toHaveClass(/font-semibold/)

  // Archiving demotes the item into the feed's collapsed archived section.
  await page.keyboard.press('e')
  await expect(item).toHaveCount(0)
  const divider = page.getByTestId('archived-divider')
  await expect(divider).toContainText('Archived (1)')
  await divider.click()
  await expect(item).toBeVisible()
  await expect(item.getByTestId('archive-reason')).toHaveText('manual')

  // Un-archiving from the archived section returns it to the active list.
  // Archiving preserves unread, so selecting the archived row runs a read
  // mutation first; wait for it to land before the next revision-guarded write.
  await item.click()
  await expect(item.getByTestId('item-title')).toHaveClass(/font-normal/)
  await page.keyboard.press('e')
  await expect(item).toBeVisible()
  await expect(item.getByTestId('archive-reason')).toHaveCount(0)
  await expect(page.getByTestId('archived-divider')).toHaveCount(0)
})

test('shows the single profile in the rail and sidebar', async ({ page }) => {
  await expect(page.getByTestId('profile-tile')).toHaveCount(1)
  await expect(page.getByTestId('sidebar-profile-name')).toHaveText('Frontend Triage')
})

test('confirms a configured action without changing the selection', async ({ page }) => {
  const detail = page.getByTestId('detail-pane')
  await expect(detail).toContainText('hive/core #2841')
  await page.getByTestId('action-card').first().click()
  await expect(page.getByTestId('toast')).toHaveText('Review PR completed')
  await expect(detail).toContainText('hive/core #2841')
})

test('opens, filters, runs, and dismisses the command palette', async ({ page }) => {
  await page.keyboard.press('Meta+k')
  const palette = page.getByTestId('command-palette')
  await expect(palette).toBeVisible()
  const input = page.getByTestId('command-palette-input')
  await input.fill('notifications')
  // The scope-prefix span and the title span are adjacent with no text node
  // between them (Vue's whitespace-condense drops the newline between the
  // two <span> tags in the template), so the row's full text has no space
  // after the ›.
  const notificationsFeed = page.getByTestId('command-palette-command').filter({ hasText: 'Frontend Triage ›Notifications inbox' })
  await expect(notificationsFeed).toBeVisible()
  await notificationsFeed.click()
  await expect(palette).toBeHidden()
  await expect(page.getByTestId('sidebar-feed').filter({ hasText: 'Notifications inbox' })).toHaveClass(/sidebar-entry-selected/)

  await page.keyboard.press('Meta+k')
  await expect(palette).toBeVisible()

  // The row just run leads Recent on the next open, ahead of its own group.
  const firstEntry = page.locator('.palette-results > *').first()
  await expect(firstEntry).toHaveClass(/palette-group-header/)
  await expect(firstEntry).toHaveText('Recent')
  // Recent rows carry the same container prefix a typed-query row does, but
  // as a separate scope span — command-palette-command-title is the title
  // alone.
  const recentRow = page.getByTestId('command-palette-command').first()
  await expect(recentRow.getByTestId('command-palette-command-title')).toHaveText('Notifications inbox')
  await expect(recentRow.getByTestId('command-palette-command-scope')).toHaveText('Frontend Triage ›')

  // A scattered query still finds a command by hopping across word starts.
  await input.fill('mkalrd')
  await expect(page.getByTestId('command-palette-command').filter({ hasText: 'Mark all as read' })).toBeVisible()

  await page.keyboard.press('Escape')
  await expect(palette).toBeHidden()

  await page.keyboard.press('Control+k')
  await expect(palette).toBeVisible()
})

test('a sigil enters its scope tab and Backspace/Tab move between them', async ({ page }) => {
  await page.keyboard.press('Meta+k')
  const palette = page.getByTestId('command-palette')
  await expect(palette).toBeVisible()
  const input = page.getByTestId('command-palette-input')
  const allTab = page.locator('[data-testid="command-palette-tab"][data-scope="all"]')
  const gotoTab = page.locator('[data-testid="command-palette-tab"][data-scope="goto"]')
  const actionsTab = page.locator('[data-testid="command-palette-tab"][data-scope="actions"]')

  // A bare "@" is absorbed: it enters the Go to scope rather than becoming
  // the first character of the query.
  await input.fill('@')
  await expect(gotoTab).toHaveClass(/palette-tab-active/)
  await expect(input).toHaveValue('')

  // Backspace on the now-empty query pops back to All.
  await input.press('Backspace')
  await expect(allTab).toHaveClass(/palette-tab-active/)

  // Tab cycles forward through the visible scopes.
  await page.keyboard.press('Tab')
  await expect(gotoTab).toHaveClass(/palette-tab-active/)
  await page.keyboard.press('Tab')
  await expect(actionsTab).toHaveClass(/palette-tab-active/)

  await page.keyboard.press('Escape')
  await expect(palette).toBeHidden()
})

test('g shows the which-key hint pill, and g s lands on Settings', async ({ page }) => {
  await page.keyboard.press('g')
  await expect(page.getByTestId('sequence-hint')).toBeVisible()

  await page.keyboard.press('s')
  await expect(page.getByTestId('sequence-hint')).toHaveCount(0)
  await expect(page.getByTestId('settings-view')).toBeVisible()
})

test('? opens the palette on the Keys tab', async ({ page }) => {
  await page.keyboard.press('?')
  const palette = page.getByTestId('command-palette')
  await expect(palette).toBeVisible()
  await expect(page.locator('[data-testid="command-palette-tab"][data-scope="keys"]')).toHaveClass(/palette-tab-active/)

  await page.keyboard.press('Escape')
  await expect(palette).toBeHidden()
})

test('a leading slash focuses the feed search box, and a key typed there does not start a sequence', async ({ page }) => {
  const search = page.getByTestId('feed-search')

  await page.keyboard.press('/')
  await expect(search).toBeFocused()

  await page.keyboard.press('g')
  await expect(search).toHaveValue('g')
  // No sequence started: the hint pill never appears, and the mode stays put.
  await expect(page.getByTestId('sequence-hint')).toHaveCount(0)
  await expect(page.getByTestId('feed-item')).toHaveCount(6)
})

test('navigates between items with j/k and the arrow keys', async ({ page }) => {
  const detail = page.getByTestId('detail-pane')
  await expect(detail).toContainText('batch_spawn: fix detached tmux env & PATH propagation')

  await page.keyboard.press('ArrowDown')
  await expect(detail).toContainText('Feed source: mirror GitHub notifications inbox')

  await page.keyboard.press('j')
  await expect(detail).toContainText('OAuth device flow for in-app GitHub auth')

  await page.keyboard.press('k')
  await expect(detail).toContainText('Feed source: mirror GitHub notifications inbox')

  await page.keyboard.press('ArrowUp')
  await expect(detail).toContainText('batch_spawn: fix detached tmux env & PATH propagation')
})

test('does not navigate while typing in the feed search box', async ({ page }) => {
  const detail = page.getByTestId('detail-pane')
  await expect(detail).toContainText('batch_spawn: fix detached tmux env & PATH propagation')

  await page.getByTestId('feed-search').focus()
  await page.keyboard.press('j')

  // The 'j' lands in the search field and must not move the selection.
  await expect(page.getByTestId('feed-search')).toHaveValue('j')
  await expect(detail).toContainText('batch_spawn: fix detached tmux env & PATH propagation')
})

test('captures a full-window screenshot', async ({ page }, testInfo) => {
  const screenshots = join(here, '..', 'screenshots')
  await mkdir(screenshots, { recursive: true })
  await page.screenshot({ path: join(screenshots, `full-window-${testInfo.project.name}.png`), fullPage: true })
})
