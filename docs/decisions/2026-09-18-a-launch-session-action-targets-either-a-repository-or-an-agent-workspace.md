# A launch-session action targets either a repository or an agent workspace

- **Status:** accepted
- **Date:** 2026-09-18

## Context

A feed item can launch a repository-backed Hive session, but some items
have no repository and belong in the agent workspace that owns the tools and
instructions for that work.

## Decision

A `launch-session` action has one optional fixed target: `repo_template` or
`workspace`. The two are mutually exclusive. An action with neither asks for
a repository or workspace when it runs.

A fixed workspace makes the action headless. Dispatch resolves the workspace
at execution time, regenerates its derived files, and starts a detached chat
with the rendered prompt. It refuses a workspace command that does not carry
`.Prompt` through `shq` (ADR the-workspace-command-is-a-template). A
workspace target cannot set `agent` (the workspace command picks the agent)
or `post_hook`.

Repository launches create Hive sessions linked through `item_session`;
workspace launches create workspace chats linked through `item_chat`.

## Consequences

- A flow action can send an item straight to an agent workspace with no git
  checkout.
- Workspace edits take effect on the next launch without reloading
  `actions.yml`. Because the action catalog and the workspace library reload
  independently, a deleted, invalid, or prompt-dropping workspace fails at
  execution, not at load.
