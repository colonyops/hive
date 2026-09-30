# First run ends in a chat with the agent instead of asking for a profile name

- **Status:** accepted
- **Date:** 2026-09-21

## Context

First run asked the user to name their first profile before anything existed
to put in it. A profile is a flow file; connecting GitHub is what fills it with
the starter feeds, and its name is visible in the rail whatever it is. The
question came too early to answer well and blocked the app until it was
answered.

The app also seeds an agent workspace named Hive on first launch, with the
skills that describe the app's own files, and nothing pointed the user at it.
Configuring profiles and feeds is the work that workspace exists for, and a
person who has just installed the app is the person least able to do it by
hand.

"First run" was inferred from "no profile exists", and the whole walk hung off
that: the Hive config step retired itself once a profile existed, and the
connect step was reached only by creating one.

## Decision

A profile always exists. `FlowsService.EnsureProfile` writes a profile named
Default when the flows directory holds no flow file, at startup and after a
delete removes the last one. A broken file counts as a profile, so a Default
is never written beside an error it would hide. First run never asks for a
name; connecting seeds the Default the same way it seeded the named one.

First run ends with a hand-off. After the notification step, a final card
offers to start a chat in the seeded Hive workspace. `StartFirstRunChat`
regenerates the workspace, then launches a detached session whose opening
message is `prompts.FirstRun`: an interview about how the person works,
followed by a proposal for their profiles and feeds that asks before writing.
The screen routes to that chat; "Not now" goes to the feed.

The seeded Hive workspace wires the `hive-desktop` and `hive-canvas` MCP
servers beside the `hive` skill package. The skills say what the app's files
mean; the servers are how the agent reads the running app and draws into a
canvas, and a workspace whose purpose is configuring Hive needs both.

First run is recorded, not inferred. `onboarding.completed` in `settings.yaml`
is written when the walk ends. With a profile always present nothing else in
the app's state says whether the hand-off happened. The steps that have their
own signal still skip themselves: a usable config is confirmed, a connected
account skips the connect card, a resolved grant skips the permission card.

## Consequences

- An existing install upgrades with no `onboarding.completed`, so it walks
  first run once: the Hive config confirmation, then the hand-off card, with
  the connected account and resolved grant skipping their cards. The connect
  step seeds only a profile with no nodes, so a replayed walk never appends a
  second starter graph.
- Deleting the last profile lands on a fresh Default, not on onboarding.
- The prompt is Go-owned (`internal/app/prompts/templates/first-run.tmpl`), as
  every agent-facing text is. Changing what the agent is asked to do is an edit
  there.
- The hand-off needs tmux and the chosen agent on PATH. When either is missing
  the card shows why and "Not now" still ends first run; the Hive workspace
  stays in Chats.
- Removing the key from `settings.yaml` walks first run again.
