import { expect, test } from '@playwright/test'

const smokePath = '/_e2e/source-to-commit'
type SmokeState = {
  claims: Array<{ externalId: string; payload: { title: string }; unread: boolean }>
  nodeRuns: Array<{ flowId: string; nodeId: string; ok: boolean; inCount: number; outCount: number; dropCount: number }>
}

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
    return { items: state.claims.length, runs: state.nodeRuns.length }
  }).toEqual({ items: 2, runs: 3 })
  const response = await page.request.get(smokePath)
  expect(response.ok()).toBeTruthy()
  const state = await response.json() as SmokeState
  expect(state.claims.map((item) => item.externalId).sort()).toEqual(['smoke-issue', 'smoke-pr'])
  expect(state.claims.map((item) => item.payload.title).sort()).toEqual(['Source-to-commit smoke PR', 'Source-to-commit smoke issue'])
  expect(state.claims.every((item) => item.unread)).toBe(true)

  // These per-node facts prove the Go event crossed the source, browser
  // worker, and feed terminal before the backend persisted its claims.
  for (const nodeId of ['fixture-source', 'worker-transform', 'smoke-feed']) {
    expect(state.nodeRuns).toContainEqual(expect.objectContaining({
      flowId: 'source-to-commit',
      nodeId,
      ok: true,
      inCount: 2,
      outCount: 2,
      dropCount: 0,
    }))
  }
})
