# A launch-session action targets either a repository or an agent workspace

- **Status:** accepted
- **Date:** 2026-09-18

## Context

A feed item can launch a repository-backed Hive session, while agent workspaces
can only be started from Chats or a schedule. Items without a repository cannot
carry their context into the workspace that owns the tools and instructions for
the work.

## Decision

A `launch-session` action has one optional fixed target: `repo_template` or
`workspace`. The two fields are mutually exclusive. An action with neither
field remains interactive and asks for a repository or workspace when it runs.

A fixed workspace makes the action headless. Dispatch resolves the current
workspace at execution time, regenerates its derived files, and starts a
detached workspace chat with the rendered action prompt. It refuses a workspace
command that does not carry `.Prompt` through `shq`. Workspace targets cannot declare
repository-only fields such as `agent` or `post_hook`.

Repository launches continue to create Hive sessions and item-session links.
Workspace launches create agent workspace session records and do not enter that
repository association.

## Consequences

A flow action node can send an item directly to an agent workspace without a
git checkout. Workspace edits take effect on the next launch without reloading
`actions.yml`. A deleted, invalid, or prompt-dropping workspace fails at
execution because the action catalog and workspace library reload independently.
