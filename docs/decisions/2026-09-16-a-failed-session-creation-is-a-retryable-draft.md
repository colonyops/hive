# A failed session creation is a retryable draft

- **Status:** accepted
- **Date:** 2026-09-16

## Context

`CreateSession` validates the New Session form and hands the rest to a
background job, because a clone takes as long as it takes. The form closed on
submit and discarded what was typed, so a failure minutes later had nowhere to
land but the job row the titlebar chip shows.

A real failure showed how little that is. A `git-lfs` shim exited 1 from a
global `post-checkout` hook, and `git clone` returns its hook's exit code, so
the checkout was complete and the clone had failed. The chip read `create hive
session: clone repository: git clone: exec git: exit status 1`, `desktop.log`
had no ERR line to anchor on, and four attempts left four 39M checkouts no
session list mentioned.

Three gaps produced that, in three places:

- `CreateOptions.Progress` names the step and was passed nothing.
- `hivecore/core/git/executor.go` clones with `_, err := e.exec.Run(...)`, and
  `pkg/executil`'s `Run` returns the child's output *with* the error. git's
  "Repository not found" was in a return value nobody read.
- the destination path and the resolved clone strategy were computed inside
  hive's `CreateSession` and appeared in neither its error nor its progress
  writer. hive *did* log both at INF ("cloning repository", with `dest=` and
  `strategy=`), so the path was in `desktop.log` all along -- uncorrelated,
  with no ERR line to anchor it to and no way for a GUI to read it.

All three were fixed upstream in `colonyops/hive` #437, which this change
vendors (`vendor.lock` -> `197dd73`, four files on top of the v0.59.0 already
here). `executil.CommandError` now carries a failed child's capped output, and
`hive.CreateSessionError` carries the failed operation, the destination and the
clone strategy.

## Decision

1. **The last failed attempt is a `SessionDraft` the form restores**, in one
   in-memory slot on `SessionsService`. `Failure` being non-nil is what says an
   attempt is waiting, so there is no second call to disagree with it. A
   duplicate name is excluded: the form is still open and the field is what is
   wrong.

2. **The durable half is an activity row.** A failure records a
   `CategorySession`/`SeverityError` event whose `metadata` carries the
   submitted form, and the Activity view offers Retry on any row carrying it.
   The slot answers "is an attempt waiting now" and dies with the process; the
   row answers "retry that one" after the toast is gone and after a restart,
   which is the case that matters when the fix is to repair a git hook first.

   The row's metadata is an existing persisted JSON column, so this needs no
   migration and no new store. The cost is that a retry lives exactly as long
   as its audit row, and that a prompt is written into the activity log. Both
   are accepted. The progress tail is left off the row: an audit entry is not
   the place for a hook's output, and the ERR line has it.

   The frontend forwards the row's metadata to the backend untouched rather
   than reading the keys, so the encoding has one owner
   (`SessionDraftMetadata` / `SessionDraftFromMetadata`).

3. **A progress writer per attempt, and its last line is the step.** Derived,
   not matched: hive is free to reword a step line, and it redirects its
   service writers at the same target, so hook output shares it.

4. **`envExecutor` returns `executil.CommandError`.** It is the desktop's own
   implementation of hive's `executil.Executor` port, so it owes the port's
   error contract: hive's git executor re-wraps only an error that is not
   already a `CommandError`, so returning one avoids a double wrap and gives
   the desktop the child's output through `errors.As` rather than a string.

5. **The failed step, the destination and the strategy come off
   `hive.CreateSessionError`, not from this app.** An earlier revision diffed
   hive's repos directory to name the checkout a failed clone left behind (~100
   lines and a heuristic) and asked hive's config for the strategy through a
   local interface. Both are deleted: hive's typed error carries all three.
   `Operation` also beats the derived progress line as the step, because it is
   the authority on which step failed rather than a guess at the last thing
   that printed.

   The progress writer stays for what the typed error does not cover: a
   failure before hive resolved a destination, and the tail, which names the
   sequence rather than one operation. The INF line on create entry stays too,
   so hive's own log lines are attributable when several creates are in
   flight.

6. **The failure is pushed, not filed.** `events.SessionCreateFailed` wakes the
   frontend, which re-reads the draft and raises an error toast that does not
   expire. The jobs chip stays as it was; it is a list the user has to go and
   read, which is the behaviour this fixes.

## Consequences

- ⌘N is the retry: `openBlank` restores the pending attempt ahead of the
  on-screen repository and the backend default, so no palette row or keybinding
  is added.
- Every hive command failing through `envExecutor` now carries its output, not
  just the clone. That is intended.
- Two concurrent creates interleave into each other's progress writers, because
  hive swaps those writers service-wide. The cost is a misattributed line in a
  diagnostic; fixing it is upstream's.
- The launcher's failure tests live in `hive_adapters_test.go`, because the
  `depguard` hivecore allowlist names that file and they construct the real
  vendored session service.
