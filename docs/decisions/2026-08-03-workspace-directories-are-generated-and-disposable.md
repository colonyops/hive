# Workspace directories are generated and disposable

- **Status:** accepted
- **Date:** 2026-08-03

## Context

Phase 3 (spec §4) gives each agent workspace a directory that mixes files a
user authors (`agent-workspace.yaml`, `AGENTS.md`, `docs/`) with files
`agentws.Generate` writes on every open (`CLAUDE.md`, `.mcp.json`,
`.codex/config.toml`, `.claude/skills/`, `.agents/skills/`, an empty `docs/`).
The workspace root is user-configurable and iCloud Drive is an expected
destination (spec §4.1) — the model has to survive syncing, eviction, and two
machines opening the same workspace, not just a single local disk.

`internal/app/skills` (ADR skill-installer) already writes generated files into a
directory the user does not own outright — `~/.claude/skills` is shared with
every other project — and earns a hash-tracked drift model that refuses to
clobber a user's hand edit as a result. Whether that same complexity is owed
here is the question this ADR answers.

## Decision

### 1. The contract: generated is disposable

Everything `Generate` writes is regenerated in full every time a workspace is
opened. It is never treated as synced state, never drift-tracked against a
previous run, and never merged with a hand edit — a hand edit to a generated
file is silently replaced on the next open. This is the intended behaviour,
not a rough edge: a generated file is a pure projection of the manifest,
`AGENTS.md`, and the resolved MCP set, so keeping a diverged copy around would
only let stale output outlive the inputs that produced it.

### 2. The explicit departure from ADR skill-installer

ADR skill-installer's skills installer hash-tracks its output and refuses to clobber a
user edit, because it writes into `~/.claude` and `~/.codex` — directories the
user shares with every other project on the machine. A hand edit there might
belong to something Hive knows nothing about, so silently overwriting it would
be destructive.

Nothing about a workspace directory shares that property. Hive owns the whole
subtree — `.claude/skills/`, `.agents/skills/`, `.codex/`, `CLAUDE.md`,
`.mcp.json`, and the empty `docs/` seed are all its output alone, with nothing
else writing there. The cheaper contract — reconcile the tree to exactly what
`Generate` computes, no hash index, no drift state to keep in step — holds
precisely because ownership is total. This is also the property that makes the
iCloud story below tractable: a drift-detecting sync has state of its own to
keep consistent across machines, and regenerate-and-compare does not.

### 3. Why prose lives in `AGENTS.md`, not in YAML

`agent-workspace.yaml` carries no `system_prompt` field. `AGENTS.md` is the
file a user (or the agent itself) will reach for to add or edit instructions,
and it is also the file every agent's own skimming already looks for. Putting
prose in a YAML string would make the obvious action — open `AGENTS.md`, type
— a no-op, since the manifest holds no such field to update and `CLAUDE.md` is
generated over whatever was there. Keeping structure in the manifest and prose
in `AGENTS.md` keeps both diffable in the form each is actually edited in.

### 4. The three iCloud failure modes and their defusals

- **Symlinks do not sync reliably in iCloud Drive.** `CLAUDE.md` is therefore
  a generated **copy** of `AGENTS.md`, never a symlink — `generateClaudeMD`
  reads `AGENTS.md` and writes `CLAUDE.md` only when the bytes differ. The
  generator is what keeps them in step; nothing depends on the filesystem
  doing it.
- **Eviction leaves `.icloud` placeholder stubs** for a cold file, and a stub
  in `docs/` is content an agent cannot read at all. For a *generated* file
  this is harmless — the next open regenerates it in place. For an *authored*
  file it is a real limitation, and the one case Hive actively handles is
  `AGENTS.md`: an evicted `.AGENTS.md.icloud` stub is detected and **reported**
  as a problem rather than treated as a missing file and generated over as an
  empty `CLAUDE.md` — losing the user's prose from the copy just because the
  original was cold would turn a sync detail into silent data loss.
- **Two machines regenerate from the same synced source independently.**
  Nothing coordinates who runs `Generate` first, so correctness depends on
  every run computing identical output from identical input: generation must
  be **deterministic byte-for-byte** — stable key ordering (`.mcp.json`'s
  `encoding/json` map-key sort, `.codex/config.toml`'s explicit id sort, no
  timestamps), and the write path (`writeIfDifferent`) only touches a file
  when its bytes actually differ. That last part is not only an optimization:
  a write iCloud does not need to re-sync is a write that cannot race a
  second machine's own regeneration of the same file.

### 5. The limitation determinism does not cover

Determinism is a property of *identical inputs*, and one enabled skill's input
is not identical across installs. The `hive-http-api` skill body interpolates
this install's loopback base URL and the absolute paths to its flows
directory, actions file, and settings file
(`http-api.tmpl:3,8,10,16-17` via `fragments.tmpl:13-15`) — every one of
those differs by username and between a dev and an installed instance. A
workspace that enables `hive-http-api` therefore writes **per-install bytes**
into a tree that may be synced to a second machine: the two machines conflict
on that file indefinitely (each open reasserts its own machine's paths), and
between opens on the same machine the file can point an agent at whatever
paths were live on whichever machine generated it last. This is stated as a
limitation in the same register as iCloud eviction — not something Hive
papers over — and it is also why a generated subtree is not something to
commit to version control alongside the authored files: doing so would commit
one machine's absolute paths as if they were portable.

### 6. codex's configuration lives in the workspace tree, not a side tree

`<workspace>/.codex/config.toml` is generated and disposable exactly like
every other file `Generate` writes — reconciled, written only when it
differs, and never hand-edited. Hive writes nothing into
`~/.codex/config.toml` and holds no trust credential of its own: codex asks
the user for directory trust itself, on first launch, keyed on the
workspace's absolute path. A workspace synced to a second machine is a
different absolute path, so codex prompts for trust there too — that is
codex's own model, not something this generator can or should shortcut.

### 7. Concurrent generation is last-writer-wins, per file

`Generate` writes a tree, not a single file, so there is no atomicity across
the set it produces — two instances (two machines, or two opens racing on one
machine) can interleave their writes. The contract is per-file atomic
replace with the last writer winning, and that is safe *because* every
generated file is a pure function of the same authored inputs: two writers
racing on the same file produce the same bytes unless the inputs themselves
changed between them, in which case the file ends up matching whichever
input was resolved last — no different from a single writer opening twice in
quick succession. `writeIfDifferent` narrows the window further by skipping
writes that would not change anything, but it does not remove the race; it is
not needed to, because nothing here depends on one writer observing the
other's write.

## Consequences

- A hand edit to any file `Generate` produces is lost on the next open. This
  must never be treated as a bug to fix — the alternative is the hash-tracked
  drift model this ADR explicitly declines, and it is only affordable in
  `~/.claude` because Hive does *not* own that whole directory.
- Adding a new generated file (a new agent's MCP wiring, say) needs no drift
  index, no migration, and no user-facing "outdated" state — it is simply a
  new entry `Generate`'s reconcile pass writes and prunes like the rest.
- Anything added to the generator must stay a pure function of
  `GenerateInput` — the clock, the environment, and the filesystem beyond what
  that struct names must never leak in, or byte-for-byte determinism (§4, §5)
  breaks silently on whichever machine happens to differ.
- `hive-http-api` cannot be treated as safe to enable in a workspace synced to
  more than one machine without accepting §5's limitation; nothing here makes
  that skill machine-portable, and a future fix (if any) belongs to the
  skill's own content, not to the generator's determinism guarantee.

## Reference

Related decisions: ADR skill-installer (the drift-tracked skill installer this
explicitly departs from, and why), ADR a-workspace-declares-its-own-authority (the autonomy model that governs
what a workspace's agent is allowed to do once it is generated), ADR go-owned-llm-prompts
(Go-owned prompts and per-type docs, the same "one declaration, consumed
everywhere" shape the MCP catalogue follows).
