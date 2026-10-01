# Contributing

[docs/development.md](docs/development.md) covers prerequisites, setup, and
how to run the hive CLI and Hive Desktop.

## Before you open a pull request

Run `mise run check`. If you changed config behavior, also run
`mise run cli:validate`. A user-visible change to either program owes a
release-notes fragment in that program's changelog
(`mise run changelog:new`; see `.agents/skills/release-notes/SKILL.md`). A
Wails service change also owes regenerated bindings (`cmd/desktop/AGENTS.md`).

The pull request template describes the title format and the scope list.

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
