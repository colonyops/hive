<!--
TITLE — Conventional Commits: type(scope): summary

  feat(workspaces): reload workspace repositories after filesystem changes
  fix(sessions): report missing agents and clean up failed spawns
  refactor(tui): fold the review modal into the view registry
  feat(config)!: key a rule by remote pattern

Imperative, lower-case after the colon, no trailing period, <= 72 chars total.
Append ! before the colon for a breaking change.

  types   feat fix perf refactor docs test build ci chore revert
  scopes  cli tui sessions msg hc config workspaces plugins sources storage
          tmux doctor
          chats canvas terminal code feed flows tasks settings actions mcp drive
          release skills telemetry tray web frontend
          docs deps repo ci

A change that genuinely spans the repo takes no scope: `refactor: ...`. If the
scope you want is missing, add it here in the same PR rather than inventing one
silently. Renovate titles are exempt — they are the bot's, not ours.
-->

<!--
If an LLM writes this pull request title, body, or any commit message on the
branch, use ASD-STE100 Simplified Technical English. Use active voice,
approved-vocabulary words, sentences of 20 words or fewer, and no gerunds.
See CONTRIBUTING.md > Writing style.

Keep every heading. Replace each comment with your answer, then delete the
comment. Write each paragraph on one line; GitHub renders a hard wrap as a
line break.

Format for a reader who scans:
- Use a bullet list for two or more separate points.
- Bold the key term at the start of a bullet: **Breaking:** ...
- Use a GitHub alert for something a reviewer must not miss, such as a
  breaking change, a manual step after merge, or a migration:
  > [!WARNING]
  > Existing agents need a relaunch.
- Describe the change at the level of design, not test by test or file by
  file. CI shows what the tests cover.
-->

## Problem

<!--
What was wrong or missing, in terms a reader who did not do the work can
picture. Name the program (hive CLI, Hive Desktop, Hive Drive Server), the
surface, and the steps or conditions that show the problem. Say what the user
saw and what they expected.

Bad:  Canvas requests could reach the installed app while the user viewed the
      agent in a development build.
Good: With the installed app and a dev build both open, an agent that ran in
      the dev build sent its canvas to the installed app. The pane in the dev
      build stayed empty.
-->

## Change

<!--
What is different after this PR, from the user's side first, then the design
decisions a reviewer needs to judge the diff. Use a bullet per decision. Do
not list files, functions, or tests. For an internal change with no user
effect, say so in one sentence.
-->

## Review notes

<!--
A bullet list of what a reviewer cannot get from the diff: tradeoffs, rejected
approaches, code that looks wrong on purpose, known limits, follow-up issues,
stacked PRs, and anything you checked by hand that CI does not cover. Say what
you are not sure about and where you want the reviewer to look. Write "None."
if there is nothing.
-->

## Checklist

<!--
Tick a box only for work this branch did. When an item does not apply, leave
it unticked and add "n/a: <reason>" after the label. An unticked box with no
reason is open work, and the PR body check fails on it.

- Unit tests: cover the new logic.
- E2E tests: cover a new or changed user flow, in cmd/desktop/e2e/tests/ or
  test/integration/.
- Docs site: the pages under docs/docs/ match the new behavior (docs-audit
  skill).
- Architecture doc: docs/architecture.md matches the new shape.
- ADR: docs/decisions/ records an architecture or infrastructure decision
  (mise run adr:new).
- Release notes: a fragment in each changed program's changelog (mise run
  changelog:new).
- Command palette: the palette and keybindings reach a new Desktop feature
  (desktop-commands-audit skill).
- Agent docs: the MCP catalog docs and shipped skills match a changed tool
  (desktop-shipped-skills skill).
- Interface parity: the CLI, TUI, and Desktop behave the same way, or a linked
  issue tracks the gap.
- Migration: a migration handles changed persisted state.
-->

- [ ] **Unit tests**
- [ ] **E2E tests**
- [ ] **Docs site**
- [ ] **Architecture doc**
- [ ] **ADR**
- [ ] **Release notes**
- [ ] **Command palette**
- [ ] **Agent docs**
- [ ] **Interface parity**
- [ ] **Migration**
- [ ] **`mise run ci` passes**

Closes #
