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

  // Step 2: authenticated with no workspaces — create the first one.
  const workspaceInput = page.getByTestId('onboarding-workspace-input')
  await expect(workspaceInput).toBeVisible({ timeout: 15_000 })
  await expect(onboarding).toContainText('Create your first workspace')
  await expect(page.getByTestId('onboarding-workspace-submit')).toBeDisabled()
  await page.screenshot({ path: join(screenshots, `onboarding-workspace-${testInfo.project.name}.png`), fullPage: true })

  await workspaceInput.fill('Frontend Triage')
  await page.getByTestId('onboarding-workspace-submit').click()

  // "New profile" seeds a real starter flow (flow.FlowStore.starterFlow —
  // three github-source -> feed pairs), but nothing has polled GitHub yet in
  // mock mode (buildPipelineProducer is skipped, and only the fixture flow
  // desktop/mockseed.go targets gets seeded feed_item rows) — so a freshly
  // created workspace starts with feeds but zero items.
  await expect(page.getByTestId('breadcrumb-profile-name')).toHaveText('Frontend Triage', { timeout: 15_000 })
  await expect(page.getByTestId('sidebar-profile-name')).toHaveText('Frontend Triage')
  await expect(page.getByTestId('sidebar-feed')).toHaveCount(3)
  await expect(page.getByTestId('feed-item')).toHaveCount(0)

  // A second profile through the rail modal — this server is ours to
  // mutate, unlike the shared feed-mode instance.
  await page.getByTestId('profile-add').click()
  const modal = page.getByTestId('new-profile-modal')
  await expect(modal).toBeVisible()
  await page.getByTestId('new-profile-input').fill('Backend Triage')
  await page.getByTestId('new-profile-submit').click()

  await expect(modal).toBeHidden()
  await expect(page.getByTestId('profile-tile')).toHaveCount(2)
  await expect(page.getByTestId('sidebar-profile-name')).toHaveText('Backend Triage')
  await expect(page.getByTestId('sidebar-feed')).toHaveCount(3)

  // ── Flows canvas: rename a starter-flow feed node, then Deploy ───────────
  // Editing a feed is done through its node in the flows canvas now (there
  // is no separate feed editor sheet). Also a mutation, so it stays on this
  // per-browser server.
  await page.getByTestId('sidebar-open-flows').click()
  const flowsView = page.getByTestId('flows-view')
  await expect(flowsView).toBeVisible()
  await expect(page.getByTestId('canvas-node-wire-count')).toHaveText('6 nodes · 3 wires')

  await page.locator('[data-testid="flow-node-my-open-prs"]').dblclick()
  const editor = page.getByTestId('node-editor')
  await expect(editor).toBeVisible()
  await expect(page.getByTestId('node-editor-title')).toHaveText('Edit node · Feed')
  await expect(page.getByTestId('node-editor-name')).toHaveValue('My open PRs')

  await page.getByTestId('node-editor-name').fill('Team PRs')
  await page.getByTestId('node-editor-save').click()
  await expect(editor).toBeHidden()
  await expect(page.getByTestId('flow-dirty-indicator')).toBeVisible()

  await page.getByTestId('deploy-button').click()
  await expect(page.getByTestId('flow-saved-indicator')).toHaveText('flows/backend-triage.yaml')
  await expect(page.getByTestId('flow-dirty-indicator')).toHaveCount(0)

  // Back to the feed view: the sidebar reflects the rename immediately
  // (Deploy's refreshFlows() re-reads the just-saved flow).
  await page.getByTestId('breadcrumb-profile-name').click()
  await expect(flowsView).toBeHidden()
  await expect(page.getByTestId('sidebar-feed')).toHaveCount(3)
  const teamRow = page.locator('[data-testid="sidebar-feed"][data-id="backend-triage/my-open-prs"]')
  await expect(teamRow).toContainText('Team PRs')

  // "Reveal in flow" (replaces the old edit pencil) jumps back into the
  // canvas, focused on the renamed node.
  await teamRow.hover()
  await teamRow.getByTestId('sidebar-reveal-in-flow').click()
  await expect(flowsView).toBeVisible()
  await expect(page.locator('[data-testid="flow-node-my-open-prs"]')).toBeVisible()
  await page.getByTestId('breadcrumb-profile-name').click()
  await expect(flowsView).toBeHidden()

  // ── Delete profile: hover reveals the header trash icon, modal confirms ───
  await expect(page.getByTestId('profile-tile')).toHaveCount(2)
  await expect(page.getByTestId('sidebar-profile-name')).toHaveText('Backend Triage')
  await page.getByTestId('sidebar-profile-header').hover()
  await page.getByTestId('sidebar-delete-profile').click()

  const deleteProfileModal = page.getByTestId('delete-profile-modal')
  await expect(deleteProfileModal).toBeVisible()
  await expect(deleteProfileModal).toContainText('Backend Triage')
  await page.getByTestId('delete-profile-confirm').click()

  await expect(deleteProfileModal).toBeHidden()
  await expect(page.getByTestId('toast').last()).toHaveText('Profile deleted')
  await expect(page.getByTestId('profile-tile')).toHaveCount(1)
  await expect(page.getByTestId('sidebar-profile-name')).toHaveText('Frontend Triage')
})
