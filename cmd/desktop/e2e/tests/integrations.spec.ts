import { expect, test } from './fixtures.js'

// The Integrations screen is a projection of the Go connector registry. What
// it used to be — a hardcoded card per connector plus a "coming soon" array —
// could drift from the registry in both directions without failing anything,
// so what these assert is the projection itself: every registered connector
// gets a card, and nothing else does.

test('lists one card per registered connector, with the mock account connected', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('feed-item')).toHaveCount(6)

  await page.getByTestId('application-settings').click()
  await page.getByTestId('settings-category-integrations').click()
  await expect(page.getByTestId('settings-integrations')).toBeVisible()

  // Every registered connector, and nothing else. Grafana is one card, not
  // two: the service groups by provider, so grafana_metrics and grafana_alerts
  // collapse into it — and posthog_errors and posthog_alerts likewise.
  await expect(page.getByTestId('integration-github')).toBeVisible()
  await expect(page.getByTestId('integration-webhook')).toBeVisible()
  await expect(page.getByTestId('integration-grafana')).toBeVisible()
  await expect(page.getByTestId('integration-exec')).toBeVisible()
  await expect(page.getByTestId('integration-posthog')).toBeVisible()
  await expect(page.getByTestId('integration-gitea')).toBeVisible()
  await expect(page.getByTestId('integration-rss')).toBeVisible()
  await expect(page.locator('[data-testid^="integration-"][data-testid$="-status"]')).toHaveCount(7)

  // The mock connection stores github/octocat, so the card reports the account
  // rather than a bare "Connected".
  await expect(page.getByTestId('integration-github-status')).toHaveText('Connected')
  await expect(page.getByTestId('integration-github')).toContainText('octocat')

  // A connector with no credential is not "not connected" — the webhook
  // listener is local ingress and reports its own runtime state instead.
  await expect(page.getByTestId('integration-webhook-status')).not.toHaveText('Not connected')

  // Nor is a connector that reads a public URL: the RSS card is listed because
  // the screen projects the registry, and it has no account to report.
  await expect(page.getByTestId('integration-rss-status')).toHaveText('Local')
})

test('the removed placeholder integrations are gone', async ({ page }) => {
  await page.goto('/')
  await page.getByTestId('application-settings').click()
  await page.getByTestId('settings-category-integrations').click()
  await expect(page.getByTestId('settings-integrations')).toBeVisible()

  // posthog was one of these until it became a registered connector; the
  // assertion above is what now covers it.
  for (const id of ['slack']) {
    await expect(page.getByTestId(`integration-${id}`)).toHaveCount(0)
  }
})

test('opens the GitHub drawer and offers to disconnect the connected account', async ({ page }) => {
  await page.goto('/')
  await page.getByTestId('application-settings').click()
  await page.getByTestId('settings-category-integrations').click()

  await page.getByTestId('integration-github-configure').click()
  await expect(page.getByTestId('github-integration-drawer')).toBeVisible()

  // Acquisition is provider-specific, so it lives here rather than on the
  // generic card.
  await expect(page.getByTestId('github-connection-account')).toContainText('octocat')
  await expect(page.getByTestId('github-connection-disconnect')).toBeVisible()

  // Disconnect drops every stored account at once, so it confirms first.
  await page.getByTestId('github-connection-disconnect').click()
  await expect(page.getByTestId('github-connection-disconnect-confirm')).toBeVisible()
  await page.getByTestId('github-connection-disconnect-cancel').click()
  await expect(page.getByTestId('github-connection-disconnect')).toBeVisible()
})
