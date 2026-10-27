# A workspace declares its own authority

- **Status:** accepted
- **Date:** 2026-08-03

## Context

An agent workspace runs a CLI agent against a named workspace with its own MCP
tool set. Coding sessions assume a disposable git clone, where a dangerous
flag's blast radius is a directory you can delete. A workspace has no such
backstop: it can hold a Home Assistant MCP server, where a tool call is a
deadbolt or a garage door, or the `hive-desktop` server, which reaches the
app's own config, actions and flows. This ADR records what authority a
workspace gets and where it comes from.

## Decision

1. **A workspace's authority is its own manifest's command, never hive's
   coding-session profiles.** The `command:` template carries the whole launch
   line, and no launch reads hive's config, so editing `hive.yaml` cannot raise
   what an existing workspace runs (ADR the-workspace-command-is-a-template).

2. **Bounded and unbounded MCP wiring is a stated tradeoff.** Claude runs with
   `--strict-mcp-config`, so its tool set is exactly the workspace's `mcps:`
   list. Codex has no equivalent: its wiring is a generated
   `.codex/config.toml`, loaded only for a project the user trusts
   interactively, and merged with the user's global codex config. Hive does not
   write codex's global config or hold a trust credential on the user's behalf.
   `MCPWiring.Bounded` (`agentws/wiring.go`) records which shape an agent gets,
   and the UI says plainly when a tool set is not limited to what the workspace
   declares.

3. **`npx -y` runs remote code on every launch.** A catalogue entry invoked
   that way fetches and executes a package at every spawn, with whatever the
   workspace's command grants it. Authority is per workspace for this reason
   too: a bypass-permissions workspace re-trusts that package's author on every
   session start.

4. **A shipped catalogue entry is an implicit endorsement.** Every shipped
   entry (`mcpcatalog/registry.go`) carries a `Stability` rating and declares
   its invocation in Go rather than accepting a command from config, and the
   catalogue shows the resolved command line for every entry, shipped or
   user-authored. Nothing launches a server whose command the user could not
   read first.

5. **Prompting needs visibility.** A command that prompts for permission only
   works if the user can see that an agent is waiting. The approval state from
   session activity (ADR agent-workspace-sessions-are-tmux-sessions) is a hard
   dependency of shipping a prompting command as the default, not a nicety.

6. **`hive-desktop` hands the agent the app, and is opt-in per workspace.**
   A workspace that lists it in `mcps:` gives its agent the same read and
   mutate surface over config, actions, feeds and flows (ADR
   mcp-replaces-the-agent-facing-http-api). That is the point of an
   orchestrator workspace, and why it is never wired into every workspace.

7. **The seeded `hive/` workspace takes that consequence deliberately.** It
   lists `hive-desktop` and `hive-drive` and runs the claude "Ask" preset, with
   no permission bypass, so nothing the seeded agent does through the app
   happens without a prompt. It is seeded when the workspace root is first
   created, and the first-run hand-off restores it if it is missing; startup
   never reseeds a deleted one.

## Consequences

- A second unbounded agent, or a bounding flag for codex, must revisit point 2.
  `MCPWiring.Bounded` is where the question is asked in code.
- Making `hive-desktop` a default for every workspace contradicts point 6 and
  needs this ADR revisited.
- Removing the approval indicator without changing the default command
  contradicts point 5.
- The terminal bearer token guards starting a workspace session, because that
  is arbitrary command execution (ADR terminal-transport).
