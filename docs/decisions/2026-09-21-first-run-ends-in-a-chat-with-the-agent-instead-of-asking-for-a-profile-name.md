# First run ends in a chat with the agent instead of asking for a profile name

- **Status:** accepted
- **Date:** 2026-09-21

## Context

Asking a new user to name a profile before anything is in it blocks the app
on a question nobody can answer well. Configuring profiles and feeds is the
work the seeded Hive agent workspace exists for, and a person who just
installed the app is the least able to do it by hand.

## Decision

**A profile always exists.** `FlowsService.EnsureProfile` writes a profile
named Default when the flows directory holds no flow file, at startup and
after a delete removes the last one. A broken file counts as a profile, so a
Default never hides its error. Connecting GitHub seeds the starter graph into
a profile that has no nodes.

**First run ends with a hand-off.** The final card offers a chat in the
seeded Hive workspace. `AgentWorkspacesService.StartFirstRunChat`
regenerates the workspace and launches a detached session whose opening
message is `prompts.FirstRun`: an interview about how the person works, then
a proposal for profiles and feeds that asks before writing. "Not now" goes
to the feed. The seeded workspace wires the `hive-desktop` and `hive-drive`
MCP servers beside the `hive` skill package.

**First run is recorded, not inferred.** `onboarding.completed` in
`settings.yaml` is written when the walk ends. Steps with their own signal
still skip themselves: a usable Hive config, a connected account, a resolved
notification grant.

## Consequences

- Removing `onboarding.completed` walks first run again. A replayed walk
  never adds a second starter graph.
- Deleting the last profile lands on a fresh Default, not on onboarding.
- The prompt is Go-owned (`cmd/desktop/internal/app/prompts/templates/first-run.tmpl`).
- When the hand-off cannot launch (no tmux, or the agent is missing), the
  card shows the error and "Not now" still ends first run.
