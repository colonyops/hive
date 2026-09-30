#!/usr/bin/env bash
set -euo pipefail

# Wires this clone's .git/hooks to lefthook. Runs from mise's postinstall hook
# and from `mise run setup`, so a fresh clone gets the gates with no manual
# step. Idempotent.

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "skip: lefthook install — not inside a git work tree" >&2
  exit 0
fi

# lefthook refuses to install whenever core.hooksPath is set, even to the
# default location, and a global core.hooksPath is a common dotfiles setting.
# Pinning this clone's local hooksPath to its own hooks dir means the --force
# below can only ever write there, never into a shared/global hooks directory.
# Trade-off: a global hooksPath's hooks do not run in this repo.
hooks_dir="$(git rev-parse --path-format=absolute --git-common-dir)/hooks"
git config --local core.hooksPath "$hooks_dir"

# lefthook's warning here names the *global* hooksPath as the target; it writes
# to the local one set above. Verify with `ls "$hooks_dir"` if in doubt.
lefthook install --force
