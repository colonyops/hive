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

Check each of these before you open a pull request.

- If you add or change logic, add unit tests. Store tests use real SQLite
  (`db.Open(t.TempDir(), ...)`).
- If you add or change a Hive Desktop user flow, add a Playwright spec in
  `cmd/desktop/e2e/tests/`.
- If you change CLI flags, stdin/stdout behavior, or a multi-command flow, add
  a test in `test/integration/`.
- If you change what a user can see, set, or do, update the pages under
  `docs/docs/`. The `docs-audit` skill finds them.
- If you change how the code is structured, update `docs/architecture.md`.
- If you make an architecture or infrastructure decision, write an ADR with
  `mise run adr:new -- "The decision, as a sentence"`.
- If users of a program will notice the change, add a release-notes fragment
  with `mise run changelog:new`.
- If you add a Hive Desktop view, object, or verb, add command palette and
  keybinding entries. The `desktop-commands-audit` skill lists them.
- If you change an MCP tool or what agents in a workspace can do, update the
  MCP catalog docs and the shipped skills (`desktop-shipped-skills` skill).
- If you change persisted data, add a migration.
- If you change config behavior, run `mise run cli:validate`.

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
