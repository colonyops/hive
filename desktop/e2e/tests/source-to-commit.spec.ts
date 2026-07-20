import { expect, test } from '@playwright/test'

const smokePath = '/_e2e/source-to-commit'
const feedID = 'source-to-commit/smoke-feed'

type SmokeState = {
  feedItems: Array<{
    feedId: string
    itemId: string
    payload: { title: string }
    unread: boolean
  }>
  nodeRuns: Array<{
    flowId: string
    nodeId: string
    ok: boolean
    inCount: number
    outCount: number
    dropCount: number
  }>
}

// This is intentionally separate from feed.spec.ts's deterministic fixture.
// It starts the normal server build in its own temp data directory, appends
// fixed source messages through Go, lets App.vue's production PipelineDriver
// run its graph (including the browser Worker function node), and finally
// reads the Go-persisted commit result through the narrow mock-only harness.
test('commits Go-appended source messages through the frontend graph', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('profile-tile')).toHaveCount(1)

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
    return { items: state.feedItems.length, runs: state.nodeRuns.length }
  }).toEqual({ items: 2, runs: 3 })

  const response = await page.request.get(smokePath)
  expect(response.ok()).toBeTruthy()
  const state = await response.json() as SmokeState

  expect(state.feedItems.map((item) => item.feedId)).toEqual([feedID, feedID])
  expect(state.feedItems.map((item) => item.itemId).sort()).toEqual(['smoke-issue', 'smoke-pr'])
  expect(state.feedItems.map((item) => item.payload.title).sort()).toEqual([
    'Source-to-commit smoke PR',
    'Source-to-commit smoke issue',
  ])
  expect(state.feedItems.every((item) => item.unread)).toBe(true)

  for (const nodeID of ['fixture-source', 'worker-transform', 'smoke-feed']) {
    expect(state.nodeRuns).toContainEqual(expect.objectContaining({
      flowId: 'source-to-commit',
      nodeId: nodeID,
      ok: true,
      inCount: 2,
      outCount: 2,
      dropCount: 0,
    }))
  }
})
