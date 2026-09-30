import { expect, test } from './fixtures.js'

// The Kind path end to end: a Go core error classified as KindNotFound,
// serialized by wailsui.MarshalError, delivered as the thrown exception's
// `cause`, and read by appErrorKind.
//
// Every other layer of this has unit coverage. What only a real RPC can prove
// is that MarshalError is actually wired into application.Options and that the
// runtime carries `cause` through — a broken wiring makes every failed call
// look unclassified, and the frontend silently stops distinguishing "the row
// is gone" from "we broke".
//
// The stale-run-id path is the one place that distinction is observable
// without an OS bus: the frontend restores persisted action run ids on load
// and asks the backend about each. An id whose output_command row does not
// exist yields KindNotFound, and the frontend must drop it silently rather
// than surface it as an unknown failure.

const runIDStorageKey = 'hive.action-run-ids'

test('a stale action run id is dropped silently rather than surfaced as a failure', async ({ page }) => {
  // A failed binding call is an HTTP 422 on the wire, and the browser logs
  // that regardless of how the app handles it — so the transport's own
  // resource-load line is filtered out. What must not appear is an
  // application-level error: a not_found run is an expected condition.
  const appConsoleErrors: string[] = []
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    if (message.text().startsWith('Failed to load resource')) return
    appConsoleErrors.push(message.text())
  })

  await page.goto('/')
  await expect(page.getByTestId('feed-item')).toHaveCount(6)

  // Run ids are keyed by the item's numeric inbox id and the action's id, so
  // both come from the running app rather than being guessed.
  const row = page.locator('[data-testid="feed-item"][data-id="pr2841"]')
  const inboxID = await row.getAttribute('data-inbox-id')
  expect(inboxID).toBeTruthy()
  await row.click()
  const actionID = await page.getByTestId('action-card').first().getAttribute('data-id')
  expect(actionID).toBeTruthy()

  // 999999 cannot exist: output_command ids start at 1 and this run seeds a
  // handful at most.
  const seeded = JSON.stringify({ [inboxID!]: { [actionID!]: 999999 } })
  await page.evaluate(([key, payload]) => window.localStorage.setItem(key, payload), [runIDStorageKey, seeded])

  await page.reload()
  await expect(page.getByTestId('feed-item')).toHaveCount(6)
  await page.locator('[data-testid="feed-item"][data-id="pr2841"]').click()
  await expect(page.getByTestId('action-card').first()).toBeVisible()

  // No failure block: a stale run surfaced rather than dropped renders one.
  await expect(page.getByTestId('action-failure')).toHaveCount(0)

  // And the stale id is forgotten, so the next load does not ask again. The
  // whole entry is pruned once its last run id goes, so the map is empty
  // rather than holding an empty object.
  await expect
    .poll(async () => page.evaluate((key) => JSON.parse(window.localStorage.getItem(key) ?? '{}'), runIDStorageKey))
    .toEqual({})

  expect(appConsoleErrors, 'a not_found run is an expected condition, not an error').toEqual([])
})
