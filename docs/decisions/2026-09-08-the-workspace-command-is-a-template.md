# The workspace command is a template

- **Status:** proposed
- **Date:** 2026-09-08

> **Supersedes ADR [a-workspace-declares-its-own-authority](2026-08-03-a-workspace-declares-its-own-authority.md)
> §1-2.** The authority ceiling those points describe -- a posture enum
> resolved through a per-agent launch table, with hive's `Flags` stripped at
> the seam -- is replaced by the command template here. Points 3-8 stand:
> the bounded/unbounded MCP split, the `npx -y` re-trust problem, the shipped
> catalogue's endorsement, the approval indicator, and what `hive-mcp` hands
> an agent are all unchanged.

## Context

A workspace declared `autonomy: ask | auto | full`, and `agentws`'s launch
table mapped `(agent, autonomy)` to that agent's flags. The table had two
entries, `claude` and `codex`.

That made the agent key a gate. A user whose hive config names a third agent
-- `pi`, or a `fable` profile pointing at a different model -- got it offered
in the workspace editor (the list comes from hive's profiles), saved a
manifest naming it, and then hit `agent "pi" has no launch mapping in this
build` when starting a session. The refusal was correct per the old design
and useless in practice: nothing about `pi` is unsafe, and there was no field
a user could edit to fix it. Every new CLI needed a Go change.

The posture enum also could not express what it claimed. Three names cannot
cover the flag vocabulary of an arbitrary CLI, and `auto` for a CLI nobody
had mapped meant nothing at all.

## Decision

### 1. The manifest carries the whole invocation

`command:` is a Go `text/template` rendered at spawn against `LaunchData`:
`.Dir`, `.MCPConfig`, `.SessionID`, `.Resume`, plus `shq` for quoting. It is
the only source of the launch line, and the only required field beside `name`.
`autonomy:` is deleted, migrated forward at manifest version 4.

`text/template` with `shq` is the shape hive already uses for session spawn
(`pkg/tmpl`, and `DefaultWindows`'s
`{{ agentCommand }} {{ agentFlags }}{{- if .Prompt }}…`), so this is the
house pattern rather than a new one. `{{ if .Resume }}` is the same
construct as that `{{- if .Prompt }}`.

The rendered command is spliced into the line **unquoted**. It already runs
under `$SHELL -l -c`, so the shell parses the author's own words -- which is
what lets a template carry an env prefix or a pipeline. Every value Hive
interpolates is quoted through `shq`. Template source is folded onto one line
before parsing, never after rendering: a newline inside an interpolated value
must survive as part of a quoted word, and folding the output would turn it
into a command separator.

### 2. There is no `agent:` field; the label is read off the command

`AgentFor` takes the command's first word, strips any directory, and
lowercases it. That label selects the activity classifier, the resume probe
and the bounded-MCP notice. A CLI this build has never heard of is a normal
workspace. `ErrUnknownAgent`, `ErrNoAutonomyMapping`, `ErrPostureUnavailable`
and `ErrCommandNotASingleWord` are all deleted.

The field was stored first and derived second, and the two orders are not
equivalent. A stored label is a copy of a word already in the command, so it
drifts the moment someone edits `command:` by hand -- and it drifts silently,
into a wrong resume probe rather than a visible error. It also made the editor
ask for the same fact twice, once as a dropdown and once inside the command
the dropdown did not constrain. Deriving it removes both. Manifest version 5
deletes the key.

Derivation is only a heuristic: `env FOO=1 claude` reads as `env`, and a
wrapper script reads as the wrapper. Both degrade the same way -- the generic
activity patterns, and a resume probe that assumes a conversation exists --
which is toward launching, never toward refusing. That is the same bet
`WiringTailFor` already made on the command word, and the cost of a stored
label is worse than the cost of a wrong guess here.

The generator writes **every** known MCP config format into every workspace
whatever the command names, so a template can point an unknown CLI at
whichever format it reads. That is what makes an unmapped agent a real
workspace rather than a degraded one.

### 3. The editor picks a command, not an agent

A workspace is created by choosing one row from a searchable list: the shipped
presets, plus one per agent profile in hive's config, plus **Custom**. Only
Custom reveals the template box and the field reference, so the common path is
one choice and never shows a brace. A saved workspace whose command matches no
row opens on Custom, which is how a hand-edited manifest stays visible rather
than hiding behind a row nobody picked.

This is the reason the label had to go rather than merely being hidden. The
old editor asked for an agent and then for a command, in that order, and
nothing tied the answers together -- a workspace labelled `codex` could run
`claude`. One control cannot produce that state.

### 4. hive's config seeds presets and nothing else

`agentCommands` now carries each profile's `Command` **and** its `Flags`,
where it used to strip them. That reverses the old §1-2 mechanism, and the
reversal is the point of this ADR, so it is worth stating exactly what
changed and what did not.

The old rule existed so a workspace could not inherit
`--dangerously-skip-permissions` from whatever a user's `hive.yaml` happened
to say. It enforced that by stripping flags at the seam and validating the
survivor as a single shell word.

What replaces it is a different property, and a stronger one: **nothing in a
launch reads hive's config at all.** A preset is copied into the manifest
once, at the moment a user picks it, into a field they are looking at. From
then on the workspace runs what its own file says. Editing `hive.yaml` cannot
change what an existing workspace launches -- which the old design did not
guarantee, since it resolved the command word out of that config on every
spawn.

So the flags cross, but only as text the user reviews, and only into a file
they own. The concern §1 named (unreviewed authority arriving from
elsewhere) is met by visibility rather than by stripping.

### 5. Danger is derived, never declared

`full` used to label itself. A free-form command cannot, so
`CommandIsDangerous` matches the command against the bypass flags this build
knows by name, and the editor warns on what is actually typed -- including a
command no preset offered. A flag not on the list produces no warning: the
answer is "not recognized", never "safe". This is weaker than an enum, and it
is the price of accepting arbitrary CLIs.

### 6. The template is validated where it is written

`Workspace.Validate` parses **and renders** the template against a probe, so a
manifest whose command cannot render is a workspace that lists with a problem,
and a bad edit is refused by the editor that made it. Under
`missingkey=error` an undefined field is an execution error, not a parse
error, so parsing alone would not have caught it. The old design had no
equivalent: a bad `(agent, autonomy)` pair only failed when someone pressed
the button.

### 7. Resume is a property of the template

`SupportsResume` renders the command both ways and compares. What matters is
whether the two launches differ, not whether `.Resume` is mentioned: a
template naming it without changing its output would relaunch an identical
line, which for a pinned session id is an error rather than a resume.

## Consequences

- A new agent CLI needs no Go change. It needs a command typed into a
  workspace, and optionally a preset and an `MCPWiring` entry later.
- A workspace's real ceiling is its own manifest. `hive.yaml` cannot raise it,
  and neither can this build's tables.
- The migration cannot read hive's config, so a profile whose command differs
  from its key (agent `fable` running `claude`) migrates to a command naming
  the key. It is visible in the editor and fixed in one edit. Agents that had
  no launch mapping migrate to their own bare name -- they never launched
  before, so nothing regresses.
- `autonomyPostureFlags` in `configmigrate` freezes version 3's table. It must
  never be re-pointed at the live preset list: two builds migrating the same
  old manifest have to produce the same command.
- Version 4 let a manifest omit `command:` and fall back to the agent's
  starter command, so the version 5 step writes the command out before
  deleting the key that produced it. It repeats the frozen table rather than
  calling the version 4 step, because two migrations sharing a helper is how
  one of them later changes the other's output.
- A wrapper command loses the claude resume probe, so a chat reopened on one
  always passes `--resume` and lets the agent print its own error. Naming the
  real CLI first (`claude`, with the wrapper's work in flags or an env prefix
  after it) is the fix, and it is a one-line edit in the field the user is
  already looking at.
- The bypass-flag list is now duplicated in the frontend, which only drives a
  warning banner. A flag added to one and not the other degrades to no
  warning, never to a wrong launch.

## Reference

Related decisions: ADR [a-workspace-declares-its-own-authority](2026-08-03-a-workspace-declares-its-own-authority.md)
(superseded in part, see above), ADR [agent-workspace-sessions-are-tmux-sessions](2026-08-03-agent-workspace-sessions-are-tmux-sessions.md)
(the tmux session a rendered line runs in), ADR [yaml-config-migration](2026-07-28-yaml-config-migration.md)
(the forward-only migration chain the version 4 and 5 steps join).
