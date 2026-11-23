# Generated CLI commands nest each word of an operation's resource as a group below a namespace

- **Status:** accepted
- **Date:** 2026-11-23

## Context

ADR every-operation-is-declared-once-as-a-spec-in-internal-ops-and-each-surface-is-built-from-it builds a CLI command from each spec as `<resource> <verb>`, with the resource in kebab case. Engine operations first landed as root groups that way: `hive task-comment create`, `hive message-read put`. A kebab name is not a namespace. People and agents expect a CLI to nest resources, and the root groups duplicated `hive hc` and `hive msg`, so retiring either spelling would break scripts.

## Decision

**A generated command sits below a namespace, and each word of the resource is a nested group.** The path is the namespace, then the words of the resource, then the verb: `diff_review.put` is `hive dt diff review put`, and `task_comment.create` is `hive hc comment create`. `opscli.Path` gives the words and the verb; the caller picks the namespace.

- **Namespaces.** Desktop operations sit below `hive dt`. Engine operations sit below the group that already owns their resource, by a table in `cmd/hive`: `task` is `hc`, `message` is `msg`, `topic` is `msg topic`, `todo` is `todo`. The table replaces the resource's first word, so a namespace does not repeat it (`hive hc list`, not `hive hc task list`).
- **One spelling.** A generated command replaces a hand-written one with the same path. Older verbs stay only as aliases (`show`, `ls`, `prune`, `pub`, `add`). A command stays hand-written only when no single operation call can do its job, such as `msg sub --listen`.
- **Flags and arguments come from the input type.** A flag is the kebab form of its field (`parentId` is `--parent-id`). The spec's `Args` are positional, in order, and the last one may be a string list that takes the rest of the words. `--input` takes the whole input as JSON (inline, `@file`, or `-` for stdin). There are no short flags.
- **Output.** A command prints JSON Lines. A command may render a human view only when stdout is a terminal, so a pipe or an agent always gets JSON and no command has a `--json` flag.
- **Collisions fail when the tree is built.** Two operations on one path, or a verb and a group with one name, are an error, and a test builds the whole tree.

## Consequences

- A new resource word can add a group level. `hive dt chat-pin put` became `hive dt chat pin put`.
- A new engine resource needs a row in the namespace table; without one the CLI panics at start and `TestEveryEngineOpHasACommand` fails.
- Hand-written flags that did not match a field name are gone (`hc create --parent`, `msg pub -t`). Scripts must use the field names.
