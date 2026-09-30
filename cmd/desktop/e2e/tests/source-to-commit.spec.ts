// fixtures.js wires the per-test /_e2e/reset: on the pipeline server it
// truncates the event log before this test's page.goto, so the freshly loaded
// frontend can never hold a checkpoint ahead of the log it reads.
import { expect, test } from './fixtures.js'

const smokePath = '/_e2e/source-to-commit'
type SmokeState = {
  claims: Array<{ externalId: string; payload: { title: string }; unread: boolean }>
  nodeRuns: Array<{ flowId: string; nodeId: string; ok: boolean; inCount: number; outCount: number; dropCount: number }>
  notifyCommands: number
}

test('commits Go-appended source messages through the flow engine', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('profile-tile')).toHaveCount(1)
  // No readiness gate: App.Start installs the engine's runners synchronously,
  // before the server can serve anything, so this append cannot race runtime
  // installation. That race is what the frontend runtime needed a
  // data-pipeline-ready marker to close.
  const append = await page.request.post(smokePath)
  expect(append.ok()).toBeTruthy()
  await expect(append.json()).resolves.toEqual({ appended: 2 })
  await expect(page.getByTestId('feed-item')).toHaveCount(2)
  await expect(page.getByTestId('feed-item').filter({ hasText: 'Source-to-commit smoke PR' })).toBeVisible()
  await expect(page.getByTestId('feed-item').filter({ hasText: 'Source-to-commit smoke issue' })).toBeVisible()
  await expect.poll(async () => {
    const response = await page.request.get(smokePath)
    expect(response.ok()).toBeTruthy()
    const state = await response.json() as SmokeState
    return { items: state.claims.length, runs: state.nodeRuns.length, notified: state.notifyCommands }
  }).toEqual({ items: 2, runs: 5, notified: 2 })
  const response = await page.request.get(smokePath)
  expect(response.ok()).toBeTruthy()
  const state = await response.json() as SmokeState
  expect(state.claims.map((item) => item.externalId).sort()).toEqual(['smoke-issue', 'smoke-pr'])
  expect(state.claims.map((item) => item.payload.title).sort()).toEqual(['Source-to-commit smoke PR', 'Source-to-commit smoke issue'])
  expect(state.claims.every((item) => item.unread)).toBe(true)

  // These per-node facts prove the event crossed the source, the function
  // node, and the feed terminal before the claims were persisted — and that
  // the dedup branch forwarded every new item to its notify terminal.
  for (const nodeId of ['fixture-source', 'worker-transform', 'smoke-feed', 'dedup-once', 'smoke-notify']) {
    expect(state.nodeRuns).toContainEqual(expect.objectContaining({
      flowId: 'source-to-commit',
      nodeId,
      ok: true,
      inCount: 2,
      dropCount: 0,
    }))
  }

  // Re-observe both items with changed payloads: the feed keeps every item
  // (same identities, updated titles) while the KV-backed dedup keeps the
  // notify branch quiet — each item interrupts once, ever.
  const rerun = await page.request.post(smokePath + '?rev=2')
  expect(rerun.ok()).toBeTruthy()
  await expect(page.getByTestId('feed-item').filter({ hasText: 'Source-to-commit smoke PR (rev 2)' })).toBeVisible()
  await expect.poll(async () => {
    const after = await page.request.get(smokePath)
    const state = await after.json() as SmokeState
    return {
      items: state.claims.length,
      notified: state.notifyCommands,
      dedupDrops: state.nodeRuns.some((run) => run.nodeId === 'dedup-once' && run.dropCount === 2),
    }
  }).toEqual({ items: 2, notified: 2, dedupDrops: true })
})
