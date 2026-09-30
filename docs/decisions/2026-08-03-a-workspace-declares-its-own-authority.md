# A workspace declares its own authority

- **Status:** accepted; [ADR agent-workspace-sessions-are-tmux-sessions](2026-08-03-agent-workspace-sessions-are-tmux-sessions.md) landed the M2 indicator point 6 made a hard dependency of shipping `ask` as a real default, and moved sessions onto tmux in the same change — a session (and a `full`-autonomy agent inside it) now outlives `App.Close`, so this ADR's authority ceiling applies for as long as the tmux session survives, not just for one Hive run
- **Date:** 2026-08-03

> **Superseded in part (2026-09-08):** points 1-2 are replaced by
> ADR [the-workspace-command-is-a-template](2026-09-08-the-workspace-command-is-a-template.md).
> The `autonomy` posture enum and its per-agent launch table are gone: a
> workspace now declares its whole invocation as a command template, `agent:`
> is a label rather than a gate, and hive's profiles seed editor presets
> instead of being stripped at a seam. Points 3-8 stand as written.

> **Superseded in part (2026-08-07):** the `experimental.agents` gate this
> feature shipped behind is deleted and the area mounts unconditionally
> (ADR terminal-agents-grafana-and-commands-graduate-out-of-experimental). Nothing else here moves — the authority ceiling in
> points 1-8 is what bounds a workspace, and it never depended on the area
> being reachable only by opt-in. The gate's one load-bearing role, that the
> agent control plane rides the terminal bearer token because starting a
> session is arbitrary command execution, is unchanged: the token is now minted
> on every run rather than only when a flag was on.

## Context

The Agents area (spec-tracked as `hc-49x3i833`) runs a CLI agent against a
named workspace with its own MCP tool set — including, once `hive-http-api`
is enabled, tools that drive Hive itself. Every existing agent surface in
this codebase assumes a coding session in a disposable git worktree, where a
dangerous flag's blast radius is a clone you can delete. A workspace has no
such backstop: it can hold a Home Assistant MCP server, where a tool call is
a deadbolt, a garage door, or an alarm, and — via `hive-http-api` — control
of the app's own config, actions, and flows. This ADR is the security
decision for that gap: what authority a workspace gets, where it comes from,
and what the shipped catalogue and the seeded workspace commit to.

## Decision

### 1. A workspace does not inherit hive's coding-session profiles

`AgentProfile` (`internal/hivecore/core/config/config.go:505-508`) carries
`Command` and `Flags`, and a profile **may** set flags such as
`--dangerously-skip-permissions` or
`--dangerously-bypass-approvals-and-sandbox`. It need not: hive's default
with no `agents:` block is `{"claude": {}}` — no flags at all
(`config.go:953-959`) — and neither string appears anywhere in this repo
today. The decision is not a claim about what any one machine's config
happens to say. It is that a workspace must not inherit whatever a user put
there, on this machine or the next: a flag defensible for a coding session in
a disposable worktree is a different proposition for an agent holding a
credential to the physical world, and nothing about a workspace's manifest
should be able to reach back through hive's config to acquire one.

### 2. What replaces it: a posture table that fails closed

A workspace declares `autonomy: ask | auto | full` in its manifest, visible
on its card in the area rather than buried in YAML (spec §7.2). `agentws`'s
launch table (`internal/app/agentws/launch.go`) maps `(agent, autonomy)` to
the per-agent flags that posture means. For claude, `ask` needs no flags at
all — the CLI's own default already prompts for anything it is not sure
of — `auto` is `--permission-mode acceptEdits` (justified in phase 5: of
claude's `--permission-mode` values, `acceptEdits` is the one that still
prompts for anything riskier than an edit, which is what "auto, but not
unattended" has to mean), and `full` is `--dangerously-skip-permissions`.
Codex's `ask` is likewise no flags, `auto` is
`--ask-for-approval on-request --sandbox workspace-write`, and `full` reaches
`--dangerously-bypass-approvals-and-sandbox`. Every failure mode here is a
refusal, never a default to the nearest neighbor: an agent key with no table
entry is `ErrUnknownAgent`; a known agent at a posture its table has no entry
for is `ErrNoAutonomyMapping`; and `auto`/`full` requested for an agent with
no known MCP wiring at all is `ErrPostureUnavailable` — that posture is
withheld rather than launched with an unbounded, unannounced tool set.
**Only `Command` crosses the seam from hive's config**
(`agentCommands` in `app.go`, dropping `Flags` at the boundary), and it is
validated as a single word (`agentws.ErrCommandNotASingleWord`) — so a flag
cannot re-enter through a command string like `claude --dangerously-skip-permissions`.
`agentCommands` — where `Flags` is dropped — is the enforcement point this
whole decision rests on.

### 3. The bounded/unbounded split is a tradeoff, not a footnote

Claude is launched with `--strict-mcp-config`, so its tool set is *exactly*
what the workspace's `mcps:` list declares — nothing from the user's own
`~/.claude.json` leaks in. Codex has no equivalent flag and no CLI-level
`--mcp-config`; its MCP wiring is a generated `<workspace>/.codex/config.toml`,
loaded only for a trusted project, with trust grantable only interactively —
there is no `--trust`-style flag and no way to express it via `-c`. So a
codex workspace's actual tool set is the workspace's declared servers
**plus** whatever the user's global codex config already has configured,
merged. That was chosen knowingly: the alternative was Hive writing to
codex's global config or holding a trust credential on the user's behalf,
which trades one workspace's isolation for a mutation with no natural
undo and a secret Hive would then have to protect. Codex prompts the user for
directory trust itself, on first launch — which is where a trust decision
belongs, not in a file Hive generates unattended. `MCPWiring.Bounded` records
which shape an agent gets so the UI can say so rather than the absence of a
flag being silently indistinguishable from "nothing else is loaded": the
workspace row (and, before any session exists, `AgentWorkspacesService`'s
own MCP notice) states plainly that an unbounded agent's tool set is not
limited to what the workspace declares.

### 4. `npx -y` runs remote code on every launch

A catalogue entry whose command is `npx -y <package>` fetches and executes a
package fresh at every spawn — that is arbitrary remote code, running with
whatever the workspace's autonomy posture grants it. This is a second,
independent reason autonomy is per-workspace rather than a single global
switch: a `full`-autonomy workspace pointed at an MCP catalogue entry is not
just trusting the entry's author once, it is re-trusting them on every
session start.

### 5. A shipped catalogue is an implicit endorsement

Shipping any MCP server declaration at all — even one — says "this is safe
enough to offer." §7.3's mitigations are what make that defensible: every
shipped entry (`playwright` is the only one at M1) carries a `Stability`
rating and pins its invocation in the Go declaration rather than accepting an
arbitrary command from config, and the catalogue view shows the *resolved*
command line for any entry, shipped or user-authored — nothing launches a
server whose command a user could not have read first.

### 6. The visibility coupling: `ask` needs the M2 indicator to be usable

`autonomy: ask` only means something if the user can tell, from the area,
that an agent is sitting at a permission prompt waiting on them — otherwise
"ask" degrades to "silently blocked" and the workspace looks broken. The M2
approval indicator (phase 8, spec §7.4) is therefore a **hard dependency** of
the autonomy model, not a nicety layered on top of it. Until it lands,
`autonomy` stays a required, explicit field with no default — a manifest
that omits it is invalid, so a workspace is never silently running at
whichever posture happens to be first in an enum.

### 7. Spec §8's consequence: `hive-http-api` hands the agent the app

A workspace that enables the `hive-http-api` skill gives its agent the same
loopback HTTP surface a human drives Hive through — config, actions, feeds,
sessions. That is not a side effect to mitigate; it is the entire point of
an *orchestrator* workspace, and it is exactly why the skill is opt-in per
workspace (`skills:` in the manifest) rather than generated into every one
generation touches.

### 8. The seeded `hive/` workspace takes that consequence deliberately

Hive ships one workspace out of the box, `hive/`, and it enables
`hive-http-api` on purpose: it is what gives a new user a surface for
configuration changes and feed curation without writing YAML first. It is
`autonomy: ask` — not incidental, but the thing that makes shipping it
defensible at all, since nothing the seeded agent can do through
`hive-http-api` happens without a prompt (subject to point 6 above: until M2
lands, that prompt is the agent CLI's own, in the pane). It is seeded only
at the moment Hive creates the workspace root for the first time, so a user
who deletes it does not get it back — the decision to keep or remove it is
theirs, once made.

## Consequences

- Every workspace's real ceiling is `agentws`'s launch table, not whatever a
  user's hive config says elsewhere — a config change to `agents:` in
  `hive.yaml` cannot silently raise a workspace's autonomy.
- The bounded/unbounded split must be re-litigated, not silently inherited,
  the day a second unbounded agent (or a bounding flag for codex) is added —
  `MCPWiring.Bounded` is the field that forces the question to be asked in
  code, not in a PR description.
- `hive-http-api` cannot be added to `.shared/skills/` (merged into every
  workspace) without revisiting this ADR: doing so would make every future
  workspace an orchestrator workspace by default, silently.
- The M2 indicator (phase 8) is not optional follow-up work; shipping `ask`
  as a real default depends on it landing, and this ADR is what a change
  quietly removing the indicator without touching autonomy semantics would be
  contradicting.

## Reference

Related decisions: ADR terminal-agents-grafana-and-commands-graduate-out-of-experimental (which retired the `experimental.agents`
gate this feature originally shipped behind, ADR terminal-experimental-gate), ADR ptyterm-terminals-are-caller-addressed (`ptyterm` caller-addressed terminals, which is how a
workspace session's PTY is addressed), ADR agent-http-api (the agent HTTP API
`hive-http-api` exposes), ADR terminal-transport (why the agent control plane is
authenticated at all — the same terminal bearer token, because starting a
session is arbitrary command execution).
