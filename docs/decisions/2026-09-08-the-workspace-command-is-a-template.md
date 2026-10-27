# The workspace command is a template

- **Status:** accepted
- **Date:** 2026-09-08

## Context

A workspace's launch line must work for any agent CLI a user names, not only
the ones this build has a table for. A posture enum (`ask | auto | full`)
mapped through a per-agent flag table made every new CLI a Go change and
could not express an arbitrary CLI's flag vocabulary. This replaces the
posture mechanism of ADR a-workspace-declares-its-own-authority (its
points 1-2); the rest of that ADR stands.

## Decision

1. **The manifest carries the whole invocation.** `command:` is a Go
   `text/template` rendered at spawn against `agentws.LaunchData`: `.Dir`,
   `.MCPConfig`, `.SessionID`, `.Resume`, `.Prompt`, with `shq` for quoting.
   It is the only source of the launch line. This is the same shape hive
   uses for session spawn (`pkg/tmpl`).

   The rendered command is spliced in **unquoted**, because it runs under
   `$SHELL -l -c` and the author's own words (an env prefix, a pipeline)
   must reach the shell. Every value Hive interpolates goes through `shq`.
   Template source is folded onto one line before parsing, never after
   rendering, so a newline inside an interpolated value stays inside a
   quoted word.

2. **There is no `agent:` field; the label is read off the command.**
   `AgentFor` takes the basename of the command's first word, lowercased. It
   selects the activity classifier, the resume probe and the bounded-MCP
   notice. A stored label would drift silently from a hand-edited command.
   A wrapper or `env FOO=1 claude` derives a label this build does not know,
   which degrades toward launching (generic activity patterns, a resume that
   assumes a conversation exists), never toward refusing. The generator
   writes every known MCP config format into every workspace, so a template
   can point any CLI at the one it reads.

3. **The editor picks a command, not an agent.** One searchable list of
   shipped presets, one preset per hive agent profile that no shipped preset
   covers, and **Custom**. Only
   Custom shows the template box. A saved command that matches no preset
   opens on Custom.

4. **hive's config seeds presets and nothing else.** A profile's preset
   carries its `Command` and `Flags`, plus the claude wiring
   (`WiringTailFor`) when the command word is `claude`. A preset is copied
   into the manifest once, when picked, into a field the user sees. No
   launch reads hive's config, so editing `hive.yaml` never changes what an
   existing workspace runs.

5. **Danger is derived, never declared.** `CommandIsDangerous` matches the
   command against the bypass flags this build knows, and the editor warns
   on what is typed. A flag not on the list gets no warning: the answer is
   "not recognized", never "safe".

6. **The template is validated where it is written.** `ValidateCommand`
   parses and renders against a probe, because under `missingkey=error` an
   undefined field fails at execution, not parse. A bad command lists as a
   workspace with a problem, and the editor refuses a bad edit.

7. **Resume and prompt support are properties of the template.**
   `SupportsResume` renders with and without `.Resume` and compares.
   `SupportsPrompt` checks that a probe prompt arrives shell-quoted. A
   command without prompt support cannot start a prompted chat or carry
   schedules.

## Consequences

- A new agent CLI needs no Go change, only a command in a workspace.
- `autonomyPostureFlags` in `configmigrate` freezes version 3's table for
  the forward migration (ADR yaml-config-migration). It must never point at
  the live preset list, and the step that drops the `agent:` key repeats it
  rather than calling the earlier step, so two builds migrating the same old
  manifest produce the same command.
- A profile whose command differs from its key migrates to a command naming
  the key. The editor shows it and one edit fixes it.
- A wrapper command loses the claude resume probe. Naming the real CLI first
  is the fix.
- The bypass-flag list is duplicated in the frontend
  (`workspace/CommandField.vue`) for the warning banner. A flag added to one
  and not the other degrades to no warning, never to a wrong launch.
