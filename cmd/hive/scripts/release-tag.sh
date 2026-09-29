#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
usage: release-tag.sh <patch|minor|major> [--dry-run]

Selects the CLI release tag for HEAD and prints "current=<tag> previous=<tag>".
When GITHUB_OUTPUT is set, it also writes the two values there.

  --dry-run   make the tag in the local repository only; push nothing

exit 0  the tag is on HEAD
exit 1  the tag is not on HEAD, or the push failed
exit 2  bad arguments, or no v* tag is reachable from HEAD
USAGE
}

# Only CLI tags count. The repository also holds tags of other programs, such
# as desktop/v1.2.3, and `git describe` without a pattern returns the nearest
# tag of any name.
readonly CLI_TAGS='v[0-9]*'

level=""
dry_run=false

for arg in "$@"; do
  case "$arg" in
    patch | minor | major) level="$arg" ;;
    --dry-run) dry_run=true ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "$level" ]]; then
  usage >&2
  exit 2
fi

bump() {
  local version="${1#v}" major minor patch
  IFS=. read -r major minor patch <<< "${version%%[-+]*}"

  case "$2" in
    major) echo "v$((major + 1)).0.0" ;;
    minor) echo "v${major}.$((minor + 1)).0" ;;
    patch) echo "v${major}.${minor}.$((patch + 1))" ;;
  esac
}

current="$(git tag --points-at HEAD --list "$CLI_TAGS" --sort=-version:refname | head -n 1)"

if [[ -n "$current" ]]; then
  # A release that failed after the tag push runs again on the same commit.
  previous="$(git describe --tags --abbrev=0 --match "$CLI_TAGS" 'HEAD^' 2> /dev/null || true)"
  echo "release-tag: HEAD already tagged $current; reusing it, bump level ignored" >&2
else
  if ! previous="$(git describe --tags --abbrev=0 --match "$CLI_TAGS" HEAD 2> /dev/null)"; then
    echo "release-tag: no v* tag reachable from HEAD" >&2
    exit 2
  fi
  current="$(bump "$previous" "$level")"

  # The local tag is necessary in a dry run too: GoReleaser reads it.
  git tag -a "$current" -m "$current" HEAD
  if [[ "$dry_run" == false ]]; then
    git push origin "refs/tags/$current"
  fi
fi

if [[ "$(git rev-list -n 1 "$current")" != "$(git rev-parse HEAD)" ]]; then
  echo "release-tag: $current is not on HEAD" >&2
  exit 1
fi

echo "current=$current previous=$previous"
if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  {
    echo "current=$current"
    echo "previous=$previous"
  } >> "$GITHUB_OUTPUT"
fi
