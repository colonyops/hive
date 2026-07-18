import { expect, test } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))

// Dedicated onboarding-mode servers, one per browser project: the mock auth
// backend is a per-process singleton that stays authenticated once the fake
// device flow grants, so projects must not share an instance. Ports match
// scripts/serve.sh.
const onboardingPorts: Record<string, number> = {
  chromium: 8932,
  webkit: 8933,
}

// One test walking the whole flow: the grant is a one-way state change on the
// server, so assertions on the pre-auth cards must happen inside this test,
// before the flow is granted.
test('first-run onboarding: token fallback card, then device flow to the feed', async ({ page }, testInfo) => {
  const port = onboardingPorts[testInfo.project.name]
  if (!port) throw new Error(`no onboarding server port for project ${testInfo.project.name}`)
  await page.goto(`http://127.0.0.1:${port}/`)

  const onboarding = page.getByTestId('onboarding')
  await expect(onboarding).toBeVisible()
  await expect(onboarding).toContainText('Triage GitHub and')
  await expect(onboarding).toContainText('Create your first workspace')
  await expect(onboarding).toContainText('Tokens are stored in your OS keychain.')
  await expect(page.getByTestId('breadcrumb-profile-name')).toBeHidden()

  // Token fallback card round-trip (no state change on the server).
  await page.getByTestId('onboarding-use-token').click()
  await expect(page.getByTestId('onboarding-token-input')).toBeVisible()
  await expect(page.getByTestId('onboarding-token-submit')).toBeDisabled()
  await page.getByTestId('onboarding-back').click()
  await expect(page.getByTestId('onboarding-connect')).toBeVisible()

  // Device flow: the mock backend grants after ~1.5s.
  await page.getByTestId('onboarding-connect').click()
  await expect(page.getByTestId('onboarding-user-code')).toHaveText('7B4C-Q22F')
  await expect(onboarding).toContainText('Waiting for authorization…')

  const screenshots = join(here, '..', 'screenshots')
  await mkdir(screenshots, { recursive: true })
  await page.screenshot({ path: join(screenshots, `onboarding-device-flow-${testInfo.project.name}.png`), fullPage: true })

  await expect(page.getByTestId('feed-item')).toHaveCount(6, { timeout: 15_000 })
  await expect(page.getByTestId('breadcrumb-profile-name')).toHaveText('Frontend Triage')
})
