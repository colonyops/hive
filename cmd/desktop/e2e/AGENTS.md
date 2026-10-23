# Hive Desktop E2E Tests

Playwright tests that drive the headless server build (`-tags server`) of Hive
Desktop in real browsers. Every server runs in mock mode inside one Docker
container, and tests run one at a time against those shared servers.

## Running Tests

```bash
mise run desktop:e2e    # Builds the image, starts the servers, runs every test
```

The suite is Docker-only. Never run Playwright or `scripts/serve.sh` on the
host: `run-docker.sh` mints a per-run harness marker that the image entrypoint
and `serve.sh` both require, so a host run cannot start the servers.

Filters go after `--` and pass through to `npx playwright test`:

```bash
mise run desktop:e2e -- --list
mise run desktop:e2e -- --grep @critical
mise run desktop:e2e -- --project features-chromium
mise run desktop:e2e -- --grep @palette --grep-invert @settings
mise run desktop:e2e -- --project features-chromium tests/features/drive/drive.spec.ts
```

The first argument must be a flag. The image entrypoint runs a first argument
that is not a flag as a command, so put a file path after a flag.

`--list` skips the servers, so it answers "what runs where" in seconds after
the image builds. The image build also runs `tsc --noEmit` over this
directory, so a broken import fails there rather than mid-suite.

| Variable             | Effect                                                               |
| -------------------- | -------------------------------------------------------------------- |
| `PW_FAIL_ON_FLAKY`   | A test that passes only on retry fails the run. Use it to hunt flakes. |
| `HIVE_E2E_IMAGE_TAG` | Image tag, so two checkouts on one host do not overwrite each other's image. |

CI sets `CI=1` in the image, which gives every test two retries. Local runs use
the same image, so they retry too. Traces of failed tests land in
`test-results/` and screenshots in `screenshots/`; both are mounted out of the
container.

`mise run ci` ends with this suite, and GitHub CI runs it in the
`desktop-e2e` job.

## Three Kinds of Test

Each kind is a directory, and each directory is one Playwright project per
browser.

| Kind      | Directory                   | Projects                                              | What goes there                                      |
| --------- | --------------------------- | ----------------------------------------------------- | ---------------------------------------------------- |
| smoke     | `tests/smoke/`              | `smoke-chromium`, `smoke-webkit`                      | The app boots and degrades cleanly: themes, diagnostics, unavailable areas |
| workflows | `tests/workflows/<server>/` | `workflows-chromium`, `workflows-onboarding-webkit`   | A long, ordered journey against a server of its own  |
| features  | `tests/features/<surface>/` | `features-chromium`, `features-webkit`                | One surface in depth against the shared feed server  |

Only the onboarding server has a webkit instance, so only
`workflows/onboarding/` runs on webkit. A new workflow runs on chromium alone
unless its server gets a webkit port too.

Keep edge cases at the lowest level that can see them. Component and store
logic belongs in Vitest (`mise run desktop:test`), engine behaviour in Go tests
or a parity fixture. A browser test covers what needs the browser and the full
stack.

## Servers

`scripts/serve.sh` starts one server per row. Each has a private data and
config root, so editor writes and watcher reloads never touch the checked-in
files.

| `server`       | Mock mode      | Browsers         | For                                                                |
| -------------- | -------------- | ---------------- | ------------------------------------------------------------------ |
| `feed`         | `feed`         | chromium, webkit | The default. Six seeded feed items, the fixture flow, a Drive space. |
| `onboarding`   | `onboarding`   | chromium, webkit | First run: nothing connected, a fake device flow that grants once. |
| `pipeline`     | `pipeline`     | chromium         | The source-to-commit flow and its `/_e2e/source-to-commit` probe.  |
| `action-smoke` | `action-smoke` | chromium         | The actions journey, with a local bare Git remote.                 |
| `action-seed`  | `action-smoke` | chromium         | Starts with no `actions.yml`, to observe first-run seeding.        |

A spec picks its server with `test.use`; `baseURL` follows from it:

```typescript
import { expect, tag, test } from '../../../fixtures/test.js'

test.describe('source to commit', { tag: [tag.flows, tag.critical] }, () => {
  test.use({ server: 'pipeline' })

  test('commits Go-appended source messages through the flow engine', async ({ page }) => {
    await page.goto('/')
  })
})
```

To reach a second server from the same test, use `serverURL(server,
browserName)` from `fixtures/servers.ts`. To add a server, add it to
`serve.sh` and `fixtures/servers.ts` together; the port lives in those two
places only.

### Resetting server state

Servers live for the whole run, so a retry would otherwise meet the rows its
failed attempt wrote. Every mock server mounts `POST /_e2e/reset`, which
restores its post-startup baseline. The `serverStateReset` option decides when
the fixture calls it:

| Mode       | When                                                                                  |
| ---------- | ------------------------------------------------------------------------------------- |
| `per-test` | The default. Reset before every test, ahead of `beforeEach` and `page.goto`.          |
| `per-file` | A serial journey whose later tests read what earlier ones wrote. Call `resetServerState()` from `beforeAll`. |
| `off`      | Onboarding. Its device-flow grant is in-memory and one-way; no reset can undo it.     |

A journey on `off` also sets `retries: 0`, because a retry would meet an
already-connected server.

## Test Data

`testdata/` holds what the servers load: flows, `actions.yml`,
`actions-smoke.yml`, the Drive files, and the terminal test image. `serve.sh`
copies it into each server's private root at startup, and fills the
`__RUN_ID__` and remote placeholders in the action-smoke copy. Edit the file
here, never a server's copy. `cmd/desktop/internal/adapter/wailsui/e2e` seeds
IDs that the fixture flow names, and a Go test there reads
`testdata/flows/frontend-triage.yaml` to keep them in step.

## Tags

Tags live in `fixtures/tags.ts`. Use `tag.*` from there, never string literals.

- Surface: `@feed`, `@palette`, `@drive`, `@flows`, `@actions`, `@terminal`,
  `@chats`, `@integrations`, `@notifications`, `@onboarding`, `@settings`,
  `@diagnostics`
- Priority: `@critical`

Put the tags on the file's `test.describe`; add a surface tag to a single test
when that test covers a second surface (the palette tests in
`features/feed/feed.spec.ts`).

An auto fixture fails any test that breaks these rules:

- Every tag exists in `fixtures/tags.ts`.
- A test has at least one surface tag.
- `@critical` appears only in `smoke/` and `workflows/`.

Every smoke and workflow test is `@critical`, so `--grep @critical` runs those
and nothing else. To add a tag, add it to `tag` and, for a surface, to
`surfaceTags`.

## Helpers and Selectors

Import `test`, `expect` and `tag` from `fixtures/test.ts`. The other helpers:

| Module                | Gives you                                                                        |
| --------------------- | -------------------------------------------------------------------------------- |
| `fixtures/servers.ts` | `serverURL(server, browserName)`                                                 |
| `fixtures/paths.ts`   | `screenshotPath(name)` and `testdataPath(...parts)`, independent of spec depth   |
| `fixtures/wails.ts`   | `bindingID(service, method)`, the numeric ID a mocked binding call must answer   |

Name screenshots with `browserName`, not the project name.

Select on `data-testid`, not on DOM structure or CSS classes. The same ids are
the agent API for the native app (the `desktop-ui-audit` skill), so renaming
one breaks both; grep `tests/` first. If an element has no id, add one to the
component.

When a second spec needs a helper, move it into `fixtures/`. Selectors and
actions for one page that several specs share go in a page object under
`pages/`.
