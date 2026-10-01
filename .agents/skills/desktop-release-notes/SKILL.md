---
name: release-notes
description: Write the release-notes line a branch owes to cmd/desktop/releasenotes/changelog/unreleased/. Use when asked to update the changelog or release notes, or to check whether the work on this branch earns an entry before the PR goes up.
---

# Add a release note

A user-visible change adds a fragment to
`cmd/desktop/releasenotes/changelog/unreleased/` **in the pull request that
earns it** (`cmd/desktop/AGENTS.md`). The fragments are the draft every dev and
beta build embeds and shows, so what you write here is the product's changelog,
not a note to a future maintainer.

One file per change is what keeps concurrent branches from conflicting over the
changelog, so **never collapse two branches' notes into one file and never
create the file by hand** -- `changelog:new` builds the name
(ADR [release-notes-accumulate-as-fragments](../../../docs/decisions/2026-09-12-release-notes-accumulate-as-fragments.md)).

A `changelog/<version>.md` is written by promotion during a release, on `main`,
by `cmd/tools/release`. Writing one on a feature branch is wrong even when the version
looks obvious, and a file naming a prerelease is rejected at parse time.

## 1. Find what the branch changed

```bash
git diff main...HEAD --stat
git log main..HEAD --format='%s%n%n%b'
```

The commit bodies carry the intent, which is most of the line already. For a
PR that is not the checked-out branch, `gh pr diff <number>` and
`gh pr view <number>`.

## 2. Decide whether it earns a note

The test is whether a user could notice without reading the diff.

Earns a note:

- a capability that did not exist -- a view, an overlay, a connector, a source,
  a node type, an MCP tool, a palette scope, a shortcut;
- behaviour that changed under someone who was already using it, including a
  default, a keybinding, or where something opens;
- a bug a user could hit, described as the symptom they saw;
- a new `settings.yaml` key or `actions.yml` field they can write.

Earns nothing:

- a refactor with no behaviour change, however large;
- tests, fixtures, mocks, e2e specs;
- CI, mise tasks, lefthook, `cmd/` development tooling;
- `docs/`, ADRs, `AGENTS.md`, agent skills;
- a change to the shared `internal/` packages or a dependency bump that changes
  nothing visible in the desktop app;
- a fix to something that never reached a build a user runs -- if the bug was
  introduced and fixed inside the same draft cycle, correct or delete the
  fragment that described it instead of adding a "Fixed" note beneath it.

## 3. Add to an existing fragment, or write a new one

Read what is already unreleased before writing:

```bash
ls cmd/desktop/releasenotes/changelog/unreleased/
```

**If a fragment already describes the exact surface you touched, edit that
fragment** so the release says what the app now does rather than listing five
days of work. A feature that lands over several PRs is one note. Two branches
editing the same fragment is a real conflict worth resolving by hand; that is
the trade, and it is rare.

Otherwise write a new one:

```bash
mise run desktop:changelog:new -- --kind added "**A notify terminal node**, so a feed can notify on new items."
```

`--kind` is `added`, `changed`, or `fixed` -- the section it renders under. For
a note that spans lines, pipe it on stdin instead of quoting it:

```bash
mise run desktop:changelog:new -- --kind fixed <<'NOTE'
**Refresh now fetches.** The feed's Refresh button and `r` re-read the
database and nothing else, so a source was only ever as fresh as the last
poll tick.
NOTE
```

The command prints the path it wrote. Read it back and edit the body or the
`kind` header if either needs work. The filename is only an ordering key, so
it does not have to match what the note ends up saying -- leave it alone.

## 4. Write it well

This is product copy a user reads inside the app. **The Simplified Technical
English rule in `CONTRIBUTING.md` applies to commit and PR text, not here** --
write these the way the committed entries in `changelog/*.md` are written.

- one fragment is one bullet: open with the thing in bold, then say what it
  does and where it is:
  `**A notify terminal node**, so a feed can notify on new items.`
- present tense, addressed to the user, describing the app rather than the
  work: "the palette knows where you are", never "we added" or "this PR";
- name the surface a user can find (`Settings ▸ Terminal`, the command palette,
  the Code view's session tree) and config keys as they are written
  (`profiles.order` in `settings.yaml`);
- for a `changed` note, say what a user has to do differently, and for a
  `fixed` note, describe the symptom, not the cause;
- mark an unfinished area `(experimental)` the way Terminal mode and Chats are;
- never name a Go package, a Vue component, an ADR, an issue, or a PR number.

Do not write a release summary. The draft has no `summary`: it describes a
whole release, which no single change can write, so promotion writes it.

## 5. Verify

```bash
go test ./internal/app/releasenotes/...
```

`TestChangelogParses` and `TestCommittedFragmentsParse` read the embedded
files, so a broken header, an unknown kind, a name the tool did not build, or
an empty body fails there rather than at release time. It runs inside
`mise run check`, which the pre-push hook already runs.
