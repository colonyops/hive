#!/usr/bin/env bash
set -euo pipefail

script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/release-tag.sh"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

# The tests must not read the git configuration of the machine.
export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.com
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.com
unset GITHUB_OUTPUT

failures=0
case_name=""

fail() {
  failures=$((failures + 1))
  echo "FAIL  $case_name: $*" >&2
}

# new_repo makes a repository with one commit, a bare "origin", and cds into it.
new_repo() {
  local dir="$workdir/$1"
  git init --quiet --bare "$dir-origin.git"
  git init --quiet --initial-branch=main "$dir"
  cd "$dir"
  git remote add origin "$dir-origin.git"
  commit "first"
}

commit() {
  git commit --quiet --allow-empty -m "$1"
}

tag() {
  git tag -a "$1" -m "$1" "${2:-HEAD}"
}

# run stores the exit code, stdout, and stderr of the script.
run() {
  set +e
  stdout="$("$script" "$@" 2> "$workdir/stderr")"
  status=$?
  set -e
  stderr="$(< "$workdir/stderr")"
}

expect_status() {
  [[ "$status" == "$1" ]] || fail "exit $status, want $1 (stderr: $stderr)"
}

expect_stdout() {
  [[ "$stdout" == "$1" ]] || fail "stdout '$stdout', want '$1'"
}

expect_stderr_has() {
  [[ "$stderr" == *"$1"* ]] || fail "stderr '$stderr' does not contain '$1'"
}

expect_origin_tags() {
  local got
  # The refs that end in ^{} are the commits that annotated tags point at.
  got="$(git ls-remote --tags origin | awk '$2 !~ /\^\{\}$/ { print $2 }' | sort | tr '\n' ' ')"
  [[ "$got" == "$1" ]] || fail "origin tags '$got', want '$1'"
}

case_name="a tag of another program is not the base version"
new_repo other-program
tag v0.1.0
commit "desktop release"
tag desktop/v0.2.0
commit "work"
run patch --dry-run
expect_status 0
expect_stdout "current=v0.1.1 previous=v0.1.0"

case_name="a tag that is not an ancestor of HEAD is not the base version"
new_repo side-branch
tag v0.1.0
git checkout --quiet -b side
commit "side work"
tag v0.5.0
git checkout --quiet main
commit "work"
run patch --dry-run
expect_status 0
expect_stdout "current=v0.1.1 previous=v0.1.0"

case_name="a tag on HEAD is used again"
new_repo rerun
tag v0.1.0
commit "work"
tag v0.1.1
run minor --dry-run
expect_status 0
expect_stdout "current=v0.1.1 previous=v0.1.0"
expect_stderr_has "reusing it"
[[ "$(git tag --list 'v*' | wc -l | tr -d ' ')" == 2 ]] || fail "the script made a new tag"

case_name="a dry run makes a local tag and pushes nothing"
new_repo dry-run
tag v0.1.0
commit "work"
run minor --dry-run
expect_status 0
expect_stdout "current=v0.2.0 previous=v0.1.0"
[[ "$(git rev-list -n 1 v0.2.0)" == "$(git rev-parse HEAD)" ]] || fail "the local tag is not on HEAD"
expect_origin_tags ""

case_name="a release pushes the tag"
new_repo release
tag v0.9.9
commit "work"
run major
expect_status 0
expect_stdout "current=v1.0.0 previous=v0.9.9"
expect_origin_tags "refs/tags/v1.0.0 "

case_name="GITHUB_OUTPUT gets the two values"
new_repo output
tag v0.1.0
commit "work"
GITHUB_OUTPUT="$workdir/github-output" run patch --dry-run
expect_status 0
[[ "$(< "$workdir/github-output")" == $'current=v0.1.1\nprevious=v0.1.0' ]] || fail "GITHUB_OUTPUT holds '$(< "$workdir/github-output")'"

case_name="no CLI tag is an error"
new_repo no-tag
tag desktop/v0.2.0
commit "work"
run patch --dry-run
expect_status 2
expect_stderr_has "no v* tag reachable from HEAD"

case_name="origin has the tag on a different commit"
new_repo conflict
tag v0.1.0
git checkout --quiet -b elsewhere
commit "other work"
tag v0.1.1
git push --quiet origin refs/tags/v0.1.1
git tag -d v0.1.1 > /dev/null
git checkout --quiet main
commit "work"
run patch
expect_status 1

case_name="a level is necessary"
new_repo no-level
tag v0.1.0
run --dry-run
expect_status 2

if [[ "$failures" -gt 0 ]]; then
  echo "$failures failed" >&2
  exit 1
fi
echo "ok"
