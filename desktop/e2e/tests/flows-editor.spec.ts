import { expect, test } from '@playwright/test'

// Additive read-only coverage of the flows editor surface (the desktop
// pipeline's Node-RED-style graph editor — see docs/source-pipeline.md).
// This suite only opens the editor and asserts it renders: no flow is
// created or deployed here (the flows/*.yaml directory the mock server
// serves is empty and deterministic, matching desktop/main.go's
// buildFlowsStore doc comment), and nothing about the existing feed-mode
// suites (config/feed-editor/feed/onboarding/theme) is touched. DOM/text
// assertions only — no new screenshot snapshots, since regenerating those
// needs the Docker-based `mise run desktop:e2e` gate.

test.beforeEach(async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('feed-item')).toHaveCount(6)
})

test('opens the flows editor from the command palette and shows the empty state', async ({ page }) => {
  await page.keyboard.press('Meta+k')
  const input = page.getByTestId('command-palette-input')
  await input.fill('flows editor')
  await expect(page.getByTestId('command-palette-command-title')).toHaveText('Open flows editor…')
  await input.press('Enter')

  const flowsView = page.getByTestId('flows-view')
  await expect(flowsView).toBeVisible()

  // No flow is selected yet (the mock server's flows directory starts
  // empty) — the picker's empty-state copy is shown instead of a canvas.
  await expect(page.getByTestId('flows-view-empty')).toBeVisible()
  await expect(page.getByTestId('flow-picker')).toBeVisible()

  // The node palette lists every registered node type, grouped by category,
  // independent of whether any flow exists yet.
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
