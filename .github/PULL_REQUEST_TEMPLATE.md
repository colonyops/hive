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
decision a reviewer needs to judge the diff. Do not list files or functions.
For an internal change with no user effect, say so in one sentence.
-->

## Review notes

<!--
Anything a reviewer cannot get from the diff: tradeoffs, rejected approaches,
code that looks wrong on purpose, known limits, follow-up issues, stacked PRs.
Say what you are not sure about and where you want the reviewer to look.
Write "None." if there is nothing.
-->

## Verification

<!--
How you know this change works. Name the tests that cover the new behavior
(file and test name) and say what each one proves. For a manual check, give the
steps and what you saw. In Hive Desktop, do manual checks through the dev MCP
servers (cmd/desktop/AGENTS.md > "Test in the running app") and name the tools
you used. Say what you did not test and why.

Do not list the gates you ran or count tests. The checklist covers the gates,
and CI shows the counts.
-->

## Checklist

<!--
Tick a box only for work this branch did. When an item does not apply, leave
it unticked and add "n/a: <reason>" at the end of the line. An unticked box
with no reason is open work, and the PR body check fails on it.
-->

- [ ] Tests: unit tests cover the new logic
- [ ] Tests: an end-to-end test covers a new or changed user flow (`cmd/desktop/e2e/tests/` or `test/integration/`)
- [ ] Docs: the hivedesktop.com pages under `docs/docs/` match the new behavior (`docs-audit` skill)
- [ ] Docs: `docs/architecture.md` matches the new shape
- [ ] ADR: `docs/decisions/` records the architecture or infrastructure decision (`mise run adr:new`)
- [ ] Release notes: a fragment in each changed program's changelog (`mise run changelog:new`)
- [ ] Desktop: the command palette and keybindings reach the new feature (`desktop-commands-audit` skill)
- [ ] Agents: the MCP catalog docs and shipped skills match the new tool behavior (`desktop-shipped-skills` skill)
- [ ] Interfaces: the CLI, TUI, and Desktop behave the same way after this change, or a linked issue tracks the gap
- [ ] Data: a migration handles changed persisted state
- [ ] Gates: `mise run ci` passes on this branch

Closes #
