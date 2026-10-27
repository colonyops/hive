#!/usr/bin/env bash
# Checks a pull request body against .github/PULL_REQUEST_TEMPLATE.md.
# Reads the body from the file argument, or from stdin:
#   gh pr view --json body -q .body | scripts/check-pr-body.sh
set -euo pipefail

if [[ $# -gt 1 ]]; then
  echo "usage: $0 [body-file]" >&2
  exit 2
fi

root=$(git rev-parse --show-toplevel)
template="$root/.github/PULL_REQUEST_TEMPLATE.md"

# A body edited in the GitHub web UI has CRLF line ends.
body=$(tr -d '\r' < "${1:-/dev/stdin}")

errors=()

while IFS= read -r heading; do
  if ! grep -qxF "$heading" <<<"$body"; then
    errors+=("missing heading: $heading")
  fi
done < <(grep -E '^## ' "$template")

if grep -qF '<!--' <<<"$body"; then
  errors+=("a template comment is still in the body; replace each comment with an answer")
fi

while IFS= read -r line; do
  errors+=("unticked item without \"n/a: <reason>\": $line")
done < <(grep -E '^- \[ \] ' <<<"$body" | grep -viF 'n/a:' || true)

if [[ ${#errors[@]} -gt 0 ]]; then
  echo "The pull request body does not follow .github/PULL_REQUEST_TEMPLATE.md:" >&2
  printf '  - %s\n' "${errors[@]}" >&2
  exit 1
fi
