# Contributing

[docs/development.md](docs/development.md) covers prerequisites, setup, and
how to run the hive CLI and Hive Desktop.

## Before you open a pull request

Run `mise run ci`. It runs every gate, including the desktop e2e suite that
GitHub CI does not run. `mise run check` is the fast Go-only subset for the
edit loop.

The pull request template has the title format, the scope list, the body
sections, and a checklist. Most of this repository's code comes from coding
agents, so the checklist makes each obligation below visible to the reviewer.
Tick an item only for work the branch did. For an item that does not apply,
write `n/a: <reason>`.

## What a change owes

| When the change... | It owes |
| --- | --- |
| adds or changes logic | Unit tests. Store tests use real SQLite (`db.Open(t.TempDir(), ...)`). |
| adds or changes a user flow in Hive Desktop | A Playwright spec in `cmd/desktop/e2e/tests/` (`mise run desktop:e2e`). |
| adds or changes CLI flag wiring, stdin/stdout, or a multi-command flow | A test in `test/integration/` (`mise run cli:integration`). |
| changes what a user can see, set, or do | A page update under `docs/docs/` (`.agents/skills/docs-audit/SKILL.md`). |
| changes the shape the architecture describes | An update to `docs/architecture.md`. It is reviewed as a spec. |
| makes an architecture or infrastructure decision | An ADR (`mise run adr:new -- "The decision, as a sentence"`). |
| is visible to users of a program | A release-notes fragment (`mise run changelog:new`; `.agents/skills/release-notes/SKILL.md`). |
| changes a Wails service surface | Regenerated bindings (`mise run desktop:bindings`; `cmd/desktop/AGENTS.md`). |
| adds a Hive Desktop view, object, or verb | Command palette and keybinding entries (`.agents/skills/desktop-commands-audit/SKILL.md`). |
| changes an MCP tool or what agents in a workspace can do | Updated MCP catalog docs and shipped skills (`.agents/skills/desktop-shipped-skills/SKILL.md`). |
| changes persisted data | A new migration. Released migrations never change (`mise run check` verifies). |
| changes config behavior | `mise run cli:validate`. |

A test proves a behavior, not a gate count. The Verification section of the
template asks which tests cover the change and what each one proves. A list of
gates that passed does not answer it.

## Writing style

This guidance applies to all contributor prose. It includes pull request
titles, pull request bodies, and commit messages.

**Human authors**: Write naturally. Use short-to-medium sentences, active
voice, and direct language. Assume that the reader knows the codebase.

**LLM authors**: Use
[ASD-STE100 Simplified Technical English](https://www.asd-ste100.org/) for all
text. Use this practical subset:

- Use words from the STE approved-vocabulary dictionary. Technical names that
  are not in the dictionary can be used as-is. Examples: `hc`, `tmux`, `mise`,
  `sqlc`, `devserver`, `lefthook`, Bubble Tea, Wails, ADR.
- Use one word for one meaning. Do not vary synonyms for style.
- Use active voice only. Do not use passive constructions.
- Use present or simple past tense. Do not use `-ing` gerunds or participles as
  nouns.
- Limit procedures to 20 words per sentence. Limit descriptions to 25 words
  per sentence.
- Put one instruction in each sentence.
- Do not use idioms, metaphors, or hedging.

The goal is text that each reviewer can understand in one read.
