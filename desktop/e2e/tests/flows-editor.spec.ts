import { expect, test } from '@playwright/test'

// Read-only coverage of the flows editor surface (the desktop pipeline's
// Node-RED-style graph editor — see docs/source-pipeline.md) against the
// mock feed server's fixture flow (desktop/e2e/fixtures/flows/
// frontend-triage.yaml). A profile IS a flow now, so the active profile's
// flow is already loaded — there is no empty state to open into; this suite
// only opens the editor and asserts the fixture's two nodes/one wire render
// correctly. No flow is created, edited, or deployed here (mutation
// coverage lives in onboarding.spec.ts, which owns the per-browser mutable
// servers). DOM/text assertions only — no new screenshot snapshots, since
// regenerating those needs the Docker-based `mise run desktop:e2e` gate.

test.beforeEach(async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('feed-item')).toHaveCount(6)
})

test('opens the flows editor from the command palette and shows the fixture flow', async ({ page }) => {
  await page.keyboard.press('Meta+k')
  const input = page.getByTestId('command-palette-input')
  await input.fill('edit flow')
  await expect(page.getByTestId('command-palette-command-title')).toHaveText('Edit flow…')
  await input.press('Enter')

  const flowsView = page.getByTestId('flows-view')
  await expect(flowsView).toBeVisible()

  // The active profile's flow (the fixture) is already selected — no empty
  // state, since a profile IS a flow.
  await expect(page.getByTestId('flows-view-empty')).toHaveCount(0)
  const canvas = page.getByTestId('flows-canvas')
  await expect(canvas).toBeVisible()
  await expect(page.getByTestId('canvas-node-wire-count')).toHaveText('2 nodes · 1 wires')

  const sourceNode = page.locator('[data-testid="flow-node-gh-source"]')
  await expect(sourceNode).toBeVisible()
  await expect(sourceNode.getByTestId('flow-node-title')).toHaveText('GitHub source')

  const feedNode = page.locator('[data-testid="flow-node-notifications-inbox"]')
  await expect(feedNode).toBeVisible()
  await expect(feedNode.getByTestId('flow-node-title')).toHaveText('Notifications inbox')

  await expect(page.getByTestId('flow-wire')).toHaveCount(1)

  // The node palette lists every registered node type, grouped by category,
  // independent of which flow is selected.
  const palette = page.getByTestId('node-palette')
  await expect(palette).toBeVisible()
  await expect(palette.getByText('Sources', { exact: true })).toBeVisible()
  await expect(palette.getByText('Process', { exact: true })).toBeVisible()
  await expect(palette.getByText('Destinations', { exact: true })).toBeVisible()

  await expect(page.locator('[data-testid="palette-entry"][data-type="github-source"]')).toBeVisible()
  await expect(page.locator('[data-testid="palette-entry"][data-type="github-filter"]')).toBeVisible()
  await expect(page.locator('[data-testid="palette-entry"][data-type="function"]')).toBeVisible()
  await expect(page.locator('[data-testid="palette-entry"][data-type="feed"]')).toBeVisible()
  await expect(page.locator('[data-testid="palette-entry"][data-type="action"]')).toBeVisible()
})

test('double-clicking a node opens its editor drawer, read-only', async ({ page }) => {
  await page.getByTestId('sidebar-open-flows').click()
  await expect(page.getByTestId('flows-view')).toBeVisible()

  await page.locator('[data-testid="flow-node-notifications-inbox"]').dblclick()

  const drawer = page.getByTestId('node-editor')
  await expect(drawer).toBeVisible()
  await expect(page.getByTestId('node-editor-title')).toHaveText('Edit node · Feed')
  await expect(page.getByTestId('node-editor-name')).toHaveValue('Notifications inbox')

  await page.keyboard.press('Escape')
  await expect(drawer).toBeHidden()
  // Escaping without Save must not mark the flow dirty.
  await expect(page.getByTestId('flow-dirty-indicator')).toHaveCount(0)
})
