# Contributing

[docs/development.md](docs/development.md) covers prerequisites, setup, and
how to run the hive CLI and Hive Desktop.

## Before you open a pull request

Run `mise run ci`. It runs every gate, including the desktop e2e suite that
GitHub CI does not run. `mise run check` is the fast Go-only subset for the
edit loop. If you changed config behavior, also run `mise run cli:validate`.

Fill in the pull request template. It has the title format, the scope list,
the body sections, and a checklist. Tick an item only for work the branch did.
For an item that does not apply, write `n/a: <reason>`. The `PR body` workflow
fails a body that is missing a section, keeps a template comment, or has an
unticked item with no reason. To check a body before you push it, run
`scripts/check-pr-body.sh <file>`.

## Keep the interfaces aligned

The hive CLI, the TUI, and Hive Desktop are three interfaces to one engine. A
change to how one of them behaves applies to all three. Put the behavior in
`internal/hive` so each interface gets it from the same code. If the change
lands in only one interface, add it to the others in the same pull request, or
open an issue for the gap and link it.

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
