import { expect, test } from './fixtures.js'

// Read-only coverage of the flows editor surface (the desktop pipeline's
// Node-RED-style graph editor — see docs/source-pipeline.md) against the
// mock feed server's fixture flow (cmd/desktop/e2e/fixtures/flows/
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

  await expect(page.locator('[data-testid="palette-entry"][data-type="sources.github"]')).toBeVisible()
  await expect(page.locator('[data-testid="palette-entry"][data-type="github-filter"]')).toBeVisible()
  await expect(page.locator('[data-testid="palette-entry"][data-type="function"]')).toBeVisible()
  await expect(page.locator('[data-testid="palette-entry"][data-type="feed"]')).toBeVisible()
  await expect(page.locator('[data-testid="palette-entry"][data-type="action"]')).toBeVisible()
})

test('double-clicking a node opens its editor drawer, read-only', async ({ page }) => {
  await page.getByTestId('sidebar-edit-flow').click()
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

// AppSelect teleports its popover to <body> precisely because the drawer body
// scrolls: an absolutely-positioned list inside it gets clipped by that
// overflow. This is the regression guard for that — the reason the flow
// editor could not just use the in-place popover the actions drawer started
// with. Read-only: the picker is opened and escaped, never chosen from.
test('the node editor select opens an unclipped, themed popover', async ({ page }) => {
  await page.getByTestId('sidebar-edit-flow').click()
  await page.locator('[data-testid="flow-node-notifications-inbox"]').dblclick()
  await expect(page.getByTestId('node-editor')).toBeVisible()

  const trigger = page.getByTestId('feed-editor-icon')
  await trigger.click()
  const popover = page.getByTestId('feed-editor-icon-popover')
  await expect(popover).toBeVisible()

  // Rendered as a child of <body>, so no ancestor's overflow can clip it, and
  // actually hit-testable at its own centre rather than painted underneath the
  // drawer.
  expect(await popover.evaluate((el) => {
    const box = el.getBoundingClientRect()
    return {
      parent: el.parentElement?.tagName,
      onTop: el.contains(document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2)),
      insideViewport: box.top >= 0 && box.left >= 0
        && box.bottom <= window.innerHeight && box.right <= window.innerWidth,
    }
  })).toEqual({ parent: 'BODY', onTop: true, insideViewport: true })

  await expect(page.getByTestId('feed-editor-icon-option-sparkles')).toBeVisible()

  await page.keyboard.press('Escape')
  await expect(popover).toHaveCount(0)
  await expect(page.getByTestId('node-editor')).toBeVisible() // the select ate the Escape, the drawer stays open
  await expect(page.getByTestId('flow-dirty-indicator')).toHaveCount(0)
})
