import { expect, test } from './fixtures.js'

// The e2e lane is the `-tags server` build, where tmuxcc compiles to the
// unavailable stub. That makes it the one place the graceful-unavailable path
// is observable end to end: TerminalsService.Available -> KindUnavailable ->
// TerminalService.Available{available:false, reason} -> the panel.
//
// What it guards is the rule that the Terminal toggle is never disabled: a
// build that cannot run tmux must explain itself inside the mode rather than
// leaving a dead button in the title bar. A regression that gates the toggle
// on availability, or that lets the unavailable probe throw instead of
// rendering, fails here.

const feedItemCount = 6

// TerminalMode.vue renders this when the availability payload carries no
// reason. Naming it here is what lets the reason assertion mean "Go supplied
// one" rather than "some text is on screen"; the exact Go copy stays unpinned.
const frontendFallbackReason = 'The terminal is not available in this build.'

test('terminal mode explains its own unavailability and hands the frame back', async ({ page }) => {
  // TerminalMode is an async component, so a failed chunk load or a throwing
  // probe shows up here rather than as a missing element. Resource-load lines
  // are filtered for the same reason errors.spec.ts filters them: the browser
  // logs those regardless of how the app handles the response.
  const appConsoleErrors: string[] = []
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    if (message.text().startsWith('Failed to load resource')) return
    appConsoleErrors.push(message.text())
  })

  await page.goto('/')
  await expect(page.getByTestId('feed-item')).toHaveCount(feedItemCount)

  const hubToggle = page.getByTestId('titlebar-mode-hub')
  const terminalToggle = page.getByTestId('titlebar-mode-terminal')
  await expect(terminalToggle).toBeEnabled()
  await expect(terminalToggle).toHaveAttribute('aria-pressed', 'false')

  await terminalToggle.click()
  await expect(terminalToggle).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByTestId('terminal-mode')).toBeVisible()

  await expect(page.getByTestId('terminal-unavailable')).toBeVisible()
  const reason = page.getByTestId('terminal-unavailable-reason')
  await expect(reason).not.toBeEmpty()
  await expect(reason).not.toHaveText(frontendFallbackReason)
  await expect(page.getByTestId('terminal-session-sidebar')).toHaveCount(0)

  // Retry re-probes rather than wedging the panel; the server build answers
  // unavailable again.
  await page.getByTestId('terminal-retry').click()
  await expect(page.getByTestId('terminal-unavailable')).toBeVisible()
  await expect(reason).not.toBeEmpty()

  // Hidden, not unmounted (ADR terminal-mode-is-hidden-not-unmounted): the pool behind the mode holds live tmux
  // clients and xterm screens, so a trip to the hub is a display flip.
  await hubToggle.click()
  await expect(page.getByTestId('terminal-mode')).toBeHidden()
  await expect(hubToggle).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByTestId('feed-item')).toHaveCount(feedItemCount)

  expect(appConsoleErrors, 'an unavailable terminal is a rendered state, not a failure').toEqual([])
})
