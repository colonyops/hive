import { expect, test, type Page } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const screenshotsDir = join(here, '..', 'screenshots')

// Dedicated onboarding-mode servers, one per browser project: the mock GitHub
// connection is a per-process singleton that stays connected once the fake
// device flow grants, so projects must not share an instance. Ports match
// scripts/serve.sh.
const onboardingPorts: Record<string, number> = {
  chromium: 8932,
  webkit: 8933,
}

// The first-run story is one ordered walk on a per-browser onboarding server.
// Splitting it into named steps pins failures to a specific transition instead
// of a line deep inside one giant test.
//
// The steps share a page and run serially because the device-flow grant is a
// one-way server state change: the group therefore opts out of retries (a
// retry would meet an already-connected server and could not replay the
// pre-connect cards). The opt-out has to cover the whole group, not just the
// connect step — Playwright retries a serial block as a unit, so re-running
// any later step replays the pre-connect ones too. Giving the post-connect
// steps retries of their own means first giving them a server that starts
// connected, which is a fixture this suite does not have. That is still true even though the app no longer gates
// on GitHub — /_e2e/reset restores durable rows, not an in-process mock
// connection. Reliability comes from the app instead: the fine-grained
// reload/bind ordering this flow exercises is covered deterministically by
// unit tests (useFeedState, useFlowsSession); this suite is the real-stack
// integration smoke on top.
test.describe.serial('first-run onboarding, then profile and flow management', () => {
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

  // Setup comes first because sessions need an agent and a repository folder.
  // The folder is typed rather than picked. The native directory dialog needs
  // a GUI, and this suite drives the headless server build, which has none;
  // the typed input exists for that reason as much as for keyboard users.
  test('starts at the hive setup step and writes a config from it', async () => {
    await page.goto('/')

    const onboarding = page.getByTestId('onboarding')
    await expect(onboarding).toBeVisible()
    await expect(onboarding).toContainText('Triage GitHub and')
    await expect(onboarding).toContainText('Set up your agent and code')
    await expect(page.getByTestId('hive-setup-agents')).toBeVisible()
    await expect(page.getByTestId('onboarding-connect')).toBeHidden()
    // No profile chrome in the title bar while onboarding (gated on profileName).
    await expect(page.getByTestId('titlebar-activity')).toBeHidden()

    // Agent availability varies in the container, so select instead of toggle.
    const claude = page.getByTestId('hive-agent-claude')
    if (await claude.getAttribute('aria-pressed') !== 'true') await claude.click()
    await expect(claude).toHaveAttribute('aria-pressed', 'true')
    await expect(page.getByTestId('onboarding-hive-submit')).toBeDisabled()

    await mkdir(screenshotsDir, { recursive: true })
    await page.screenshot({ path: join(screenshotsDir, `onboarding-hive-${projectName}.png`), fullPage: true })

    // /tmp exists in the container, is not a repository, and should report no
    // repositories of its own.
    await page.getByTestId('hive-workspace-path').fill('/tmp')
    await page.getByTestId('hive-workspace-path-add').click()
    await expect(page.getByTestId('hive-workspace-list')).toContainText('/tmp')
    await expect(page.getByTestId('onboarding-hive-submit')).toBeEnabled()

    await page.getByTestId('onboarding-hive-submit').click()

    await expect(page.getByTestId('onboarding-connect')).toBeVisible({ timeout: 15_000 })
    await expect(onboarding).toContainText('Connect to GitHub')
    await expect(onboarding).toContainText('Tokens are stored in your OS keychain.')
  })

  test('offers the token fallback and warns before the connect step can be skipped', async () => {
    // Token fallback card round-trip (no state change on the server).
    await page.getByTestId('onboarding-use-token').click()
    await expect(page.getByTestId('onboarding-token-input')).toBeVisible()
    await expect(page.getByTestId('onboarding-token-submit')).toBeDisabled()
    await page.getByTestId('onboarding-back').click()
    await expect(page.getByTestId('onboarding-connect')).toBeVisible()

    // Bypassing is possible, but only past the warning — and the warning is
    // backable-out-of. This walk backs out rather than taking it: confirming
    // spends the one-way device-flow grant this server exists to exercise, and
    // taking it and coming back needs a delete-and-recreate detour that made
    // the later deploy step race its own flows:updated under load. The
    // skip-through, and the empty state it lands on, are pinned deterministically
    // in App.spec.ts instead.
    await expect(page.getByTestId('onboarding-skip')).toHaveText('Continue without GitHub')
    await page.getByTestId('onboarding-skip').click()
    await expect(page.getByTestId('onboarding')).toContainText('Skip connecting GitHub?')
    await expect(page.getByTestId('onboarding')).toContainText('Settings ▸ Integrations')
    await page.getByTestId('onboarding-skip-back').click()
    await expect(page.getByTestId('onboarding-connect')).toBeVisible()
  })

  test('grants through the device flow, which seeds the profile it made', async () => {
    // Device flow: the mock backend grants after ~1.5s.
    await page.getByTestId('onboarding-connect').click()
    await expect(page.getByTestId('onboarding-user-code')).toHaveText('7B4C-Q22F')
    await expect(page.getByTestId('onboarding')).toContainText('Waiting for authorization…')
    await page.screenshot({ path: join(screenshotsDir, `onboarding-device-flow-${projectName}.png`), fullPage: true })

    // A headless run can neither grant OS permission nor rely on an agent
    // binary, so it skips both steps.
    await expect(page.getByTestId('onboarding-permissions-skip')).toHaveText('Not now')
    await page.getByTestId('onboarding-permissions-skip').click()
    await expect(page.getByTestId('onboarding-agent-start')).toBeVisible()
    await page.screenshot({ path: join(screenshotsDir, `onboarding-agent-${projectName}.png`), fullPage: true })
    await page.getByTestId('onboarding-agent-skip').click()

    // The starter graph is three sources.github -> feed pairs plus a "Review
    // requests" feed and notify node behind a filter. Mock mode skips polling,
    // and the e2e seeder targets another flow, so these feeds start empty.
    await expect(page.getByTestId('sidebar-profile-name')).toHaveText('Default', { timeout: 15_000 })
    await expect(page.getByTestId('sidebar-feed')).toHaveCount(4)
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
    await expect(page.getByTestId('sidebar-feed')).toHaveCount(4)
  })

  test('renames a feed node in the flows canvas and deploys it', async () => {
    // Editing a feed is done through its node in the flows canvas now (there is
    // no separate feed editor sheet).
    await page.getByTestId('sidebar-edit-flow').click()
    const flowsView = page.getByTestId('flows-view')
    await expect(flowsView).toBeVisible()
    // Three source→feed pairs, plus the seeded "Review requests" feed and
    // notify node behind a filter on the notifications source.
    await expect(page.getByTestId('canvas-node-wire-count')).toHaveText('9 nodes · 6 wires')
    await expect(page.locator('[data-testid="flow-node-review-requests"]')).toBeVisible()

    await page.locator('[data-testid="flow-node-my-open-prs"]').dblclick()
    const editor = page.getByTestId('node-editor')
    await expect(editor).toBeVisible()
    await expect(page.getByTestId('node-editor-title')).toHaveText('Edit node · Feed')
    await expect(page.getByTestId('node-editor-name')).toHaveValue('My open PRs')

    await page.getByTestId('node-editor-name').fill('Team PRs')
    // Pin the edit before Save reads it. A fill that has not landed would save
    // the old name and surface two steps later as a stale sidebar label, which
    // points at the wrong subsystem entirely.
    await expect(page.getByTestId('node-editor-name')).toHaveValue('Team PRs')
    await page.getByTestId('node-editor-save').click()
    await expect(editor).toBeHidden()
    await expect(page.getByTestId('flow-dirty-indicator')).toBeVisible()

    await page.getByTestId('deploy-button').click()
    await expect(page.getByTestId('flow-saved-indicator')).toHaveText('flows/backend-triage.yaml')
    await expect(page.getByTestId('flow-dirty-indicator')).toHaveCount(0)
  })

  test('the sidebar reflects the deployed rename', async () => {
    // Back to the feed view via the spaces rail (the title-bar breadcrumb is
    // gone). Deploy publishes flows:updated, and returning to the already-active
    // profile re-reads its feeds (App.vue's requestSelectProfile), so the new
    // label is expected without a second trip through the canvas.
    await page.locator('[data-testid="profile-tile"][data-id="backend-triage"]').click()
    await expect(page.getByTestId('flows-view')).toBeHidden()
    // A shape check, not a synchronisation point — there were already four
    // feeds before the rename, so this cannot wait for the new label to land.
    // The assertion below is the one that has to converge on its own, and it is
    // deliberately not helped along: a step between it and the deploy would
    // give the sidebar a second read and hide whether one was enough.
    await expect(page.getByTestId('sidebar-feed')).toHaveCount(4)
    const teamRow = page.locator('[data-testid="sidebar-feed"][data-id="backend-triage/my-open-prs"]')
    await expect(teamRow).toContainText('Team PRs')
  })

  test('the Edit flow footer returns to the canvas holding the renamed node', async () => {
    // (There is no per-feed reveal-in-flow icon anymore — a feed is edited by
    // opening its node in the canvas.) A rename does not change the node id, so
    // it is still addressed as flow-node-my-open-prs; assert the name it now
    // carries rather than mere presence, which a failed deploy would also pass.
    const flowsView = page.getByTestId('flows-view')
    await page.getByTestId('sidebar-edit-flow').click()
    await expect(flowsView).toBeVisible()
    await expect(page.locator('[data-testid="flow-node-my-open-prs"]')).toContainText('Team PRs')
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
    await expect(page.getByTestId('sidebar-profile-name')).toHaveText('Default')
  })
})
