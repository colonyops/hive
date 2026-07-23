import { expect, test, type Page } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const screenshotsDir = join(here, '..', 'screenshots')

// Dedicated onboarding-mode servers, one per browser project: the mock auth
// backend is a per-process singleton that stays authenticated once the fake
// device flow grants, so projects must not share an instance. Ports match
// scripts/serve.sh.
const onboardingPorts: Record<string, number> = {
  chromium: 8932,
  webkit: 8933,
}

// The first-run story is one ordered walk on a per-browser onboarding server.
// It used to be a single ~70-assertion test; splitting it into named steps
// that share one page keeps the exact same end-to-end coverage but pins any
// failure to a specific step (auth vs. workspace-create vs. flow-edit vs.
// delete) instead of a line deep inside one giant test.
//
// The steps share a page and run serially because the device-flow grant is a
// one-way server state change: the group therefore opts out of retries (a
// retry would meet an already-authenticated server and could not replay the
// pre-auth cards). Reliability comes from the app instead — the fine-grained
// reload/bind ordering this flow exercises is covered deterministically by unit
// tests (useFeedState, useFlowsSession); this suite is the real-stack
// integration smoke on top.
test.describe.serial('first-run onboarding, then workspace and flow management', () => {
  test.describe.configure({ retries: 0 })

  let page: Page
  let projectName: string

  test.beforeAll(async ({ browser }, testInfo) => {
    projectName = testInfo.project.name
    const port = onboardingPorts[projectName]
    if (!port) throw new Error(`no onboarding server port for project ${projectName}`)
    page = await browser.newPage({
      baseURL: `http://127.0.0.1:${port}`,
      viewport: { width: 1360, height: 864 },
    })
  })

  test.afterAll(async () => {
    await page.close()
  })

  test('shows the onboarding cards, then grants through the device flow', async () => {
    await page.goto('/')

    const onboarding = page.getByTestId('onboarding')
    await expect(onboarding).toBeVisible()
    await expect(onboarding).toContainText('Triage GitHub and')
    await expect(onboarding).toContainText('Create your first workspace')
    await expect(onboarding).toContainText('Tokens are stored in your OS keychain.')
    // No profile chrome in the title bar while onboarding (gated on profileName).
    await expect(page.getByTestId('titlebar-activity')).toBeHidden()

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

    await mkdir(screenshotsDir, { recursive: true })
    await page.screenshot({ path: join(screenshotsDir, `onboarding-device-flow-${projectName}.png`), fullPage: true })
  })

  test('creates the first workspace and lands on an empty feed', async () => {
    // Authenticated with no workspaces — create the first one. "New profile"
    // seeds a real starter flow (flow.FlowStore.starterFlow — three
    // github-source -> feed pairs), but nothing has polled GitHub yet in mock
    // mode (buildPipelineProducer is skipped, and only the fixture flow
    // desktop/mockseed.go targets gets seeded feed_item rows) — so a freshly
    // created workspace starts with feeds but zero items.
    const workspaceInput = page.getByTestId('onboarding-workspace-input')
    await expect(workspaceInput).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('onboarding')).toContainText('Create your first workspace')
    await expect(page.getByTestId('onboarding-workspace-submit')).toBeDisabled()
    await page.screenshot({ path: join(screenshotsDir, `onboarding-workspace-${projectName}.png`), fullPage: true })

    await workspaceInput.fill('Frontend Triage')
    await page.getByTestId('onboarding-workspace-submit').click()

    await expect(page.getByTestId('sidebar-profile-name')).toHaveText('Frontend Triage', { timeout: 15_000 })
    await expect(page.getByTestId('sidebar-feed')).toHaveCount(3)
    await expect(page.getByTestId('feed-item')).toHaveCount(0)
  })

  test('adds a second profile through the rail modal', async () => {
    // This server is ours to mutate, unlike the shared feed-mode instance.
    await page.getByTestId('profile-add').click()
    const modal = page.getByTestId('new-profile-modal')
    await expect(modal).toBeVisible()
    await page.getByTestId('new-profile-input').fill('Backend Triage')
    await page.getByTestId('new-profile-submit').click()

    await expect(modal).toBeHidden()
    await expect(page.getByTestId('profile-tile')).toHaveCount(2)
    await expect(page.getByTestId('sidebar-profile-name')).toHaveText('Backend Triage')
    await expect(page.getByTestId('sidebar-feed')).toHaveCount(3)
  })

  test('renames a feed node in the flows canvas, deploys, and the sidebar reflects it', async () => {
    // Editing a feed is done through its node in the flows canvas now (there is
    // no separate feed editor sheet).
    await page.getByTestId('sidebar-edit-flow').click()
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

    // Back to the feed view via the spaces rail (the title-bar breadcrumb is
    // gone): the sidebar reflects the rename (Deploy's flows:updated re-reads
    // the just-saved flow).
    await page.locator('[data-testid="profile-tile"][data-id="backend-triage"]').click()
    await expect(flowsView).toBeHidden()
    await expect(page.getByTestId('sidebar-feed')).toHaveCount(3)
    const teamRow = page.locator('[data-testid="sidebar-feed"][data-id="backend-triage/my-open-prs"]')
    await expect(teamRow).toContainText('Team PRs')

    // The "Edit flow" footer jumps back into the canvas, where the renamed node
    // lives. (There is no per-feed reveal-in-flow icon anymore — a feed is
    // edited by opening its node in the canvas.)
    await page.getByTestId('sidebar-edit-flow').click()
    await expect(flowsView).toBeVisible()
    await expect(page.locator('[data-testid="flow-node-my-open-prs"]')).toBeVisible()
    await page.locator('[data-testid="profile-tile"][data-id="backend-triage"]').click()
    await expect(flowsView).toBeHidden()
  })

  test('deletes a profile from its routed settings danger zone', async () => {
    await expect(page.getByTestId('profile-tile')).toHaveCount(2)
    await expect(page.getByTestId('sidebar-profile-name')).toHaveText('Backend Triage')
    await page.getByTestId('sidebar-open-settings').click()
    await page.getByTestId('profile-settings-danger').click()
    await page.getByTestId('profile-settings-delete').click()

    const deleteProfileModal = page.getByTestId('delete-profile-modal')
    await expect(deleteProfileModal).toBeVisible()
    await expect(deleteProfileModal).toContainText('Backend Triage')
    await page.getByTestId('delete-profile-confirm').click()

    await expect(deleteProfileModal).toBeHidden()
    await expect(page.getByTestId('toast').last()).toContainText('Profile deleted')
    await expect(page.getByTestId('profile-tile')).toHaveCount(1)
    await expect(page.getByTestId('sidebar-profile-name')).toHaveText('Frontend Triage')
  })
})
