import { expect, test } from './fixtures.js'

// The e2e lane is the `-tags server` build, where ptyterm's buildSupportsTerminal
// compiles to false (cmd/desktop/internal/app/ptyterm/supported_server.go). That makes it
// the one place the graceful-unavailable path is observable end to end for
// the Chats area too: AgentWorkspacesService.Available -> KindUnavailable ->
// AgentsService.Available{available:false, reason} -> the mode.
//
// What it guards is the "the toggle is never disabled" rule, proven for a
// third segment: a build that cannot run a PTY at all must explain itself
// inside the Chats area rather than leaving a dead button in the title bar.
// This is the only agent path observable in the server build — starting a
// session needs a live PTY this build does not have — so nothing else
// exercises the unavailable branch.

const feedItemCount = 6

// AgentsMode.vue renders this when the availability payload carries no
// reason. Naming it here is what lets the reason assertion mean "Go supplied
// one" rather than "some text is on screen"; the exact Go copy stays unpinned.
const frontendFallbackReason = 'The Chats area is not available in this build.'

test('the Chats area explains its own unavailability and hands the frame back', async ({ page }) => {
  // AgentsMode is an async component, so a failed chunk load or a throwing
  // probe shows up here rather than as a missing element.
  const appConsoleErrors: string[] = []
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    if (message.text().startsWith('Failed to load resource')) return
    appConsoleErrors.push(message.text())
  })

  await page.goto('/')
  await expect(page.getByTestId('feed-item')).toHaveCount(feedItemCount)

  const hubToggle = page.getByTestId('titlebar-mode-hub')
  const agentsToggle = page.getByTestId('titlebar-mode-agents')
  await expect(agentsToggle).toBeEnabled()
  await expect(agentsToggle).toHaveAttribute('aria-pressed', 'false')

  await agentsToggle.click()
  await expect(agentsToggle).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByTestId('agents-mode')).toBeVisible()

  await expect(page.getByTestId('agents-unavailable')).toBeVisible()
  const reason = page.getByTestId('agents-unavailable-reason')
  await expect(reason).not.toBeEmpty()
  await expect(reason).not.toHaveText(frontendFallbackReason)
  await expect(page.getByTestId('agents-workspace-sidebar')).toHaveCount(0)

  // Retry re-probes rather than wedging the panel; the server build answers
  // unavailable again.
  await page.getByTestId('agents-retry').click()
  await expect(page.getByTestId('agents-unavailable')).toBeVisible()
  await expect(reason).not.toBeEmpty()

  // Hidden, not unmounted (ADR terminal-mode-is-hidden-not-unmounted): a trip to the hub is a display flip.
  await hubToggle.click()
  await expect(page.getByTestId('agents-mode')).toBeHidden()
  await expect(hubToggle).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByTestId('feed-item')).toHaveCount(feedItemCount)

  expect(appConsoleErrors, 'an unavailable Chats area is a rendered state, not a failure').toEqual([])
})

// The full-page canvas view reads canvases through the Chats area's client, so
// this build has none to show. The overlay still has to open on its chord,
// say why, and hand the feed back.
test('the Canvases view opens over the feed and explains its own unavailability', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('feed-item')).toHaveCount(feedItemCount)

  await page.keyboard.press('ControlOrMeta+Shift+P')
  await expect(page.getByTestId('canvas-overlay')).toBeVisible()
  await expect(page.getByTestId('canvas-page-unavailable')).not.toBeEmpty()
  await expect(page.getByTestId('canvas-page-sidebar')).toHaveCount(0)

  await page.keyboard.press('Escape')
  await expect(page.getByTestId('canvas-overlay')).toHaveCount(0)
  await expect(page.getByTestId('feed-item')).toHaveCount(feedItemCount)
})
