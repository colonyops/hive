#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
usage: scripts/github-labels.sh [--dry-run] [--prune] [--repo owner/name]
  --dry-run   print the create/update (and, with --prune, delete) plan with the
              issue count per label to delete; change nothing
  --prune     also delete repo labels not in .github/labels.yml
  --repo      default: gh repo view --json nameWithOwner
exit 0 on success; non-zero on the first gh failure
USAGE
}

dry_run=false
prune=false
repo=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) dry_run=true ;;
    --prune) prune=true ;;
    --repo)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      repo="$2"
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
  shift
done

labels_file="$(git rev-parse --show-toplevel)/.github/labels.yml"
[[ -n "$repo" ]] || repo="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
declared="$workdir/declared.tsv"
current="$workdir/current.tsv"

yq -r '.[] | [.name, (.color | tostring | downcase), .description // ""] | @tsv' "$labels_file" > "$declared"
gh label list --repo "$repo" --limit 1000 --json name,color,description \
  --jq '.[] | [.name, (.color | ascii_downcase), .description // ""] | @tsv' > "$current"

invalid="$(awk -F'\t' '$2 !~ /^[0-9a-f]{6}$/ { print "  " $1 ": " $2 }' "$declared")"
if [[ -n "$invalid" ]]; then
  printf 'invalid color in %s:\n%s\n' "$labels_file" "$invalid" >&2
  exit 1
fi

# GitHub label names are case-insensitive, so every name comparison is too.
# A case-sensitive match would make --prune delete a label that is declared
# with different capitals.
lookup() {
  NAME="$1" awk -F'\t' 'tolower($1) == tolower(ENVIRON["NAME"]) { print $2 "\t" $3; found = 1 } END { exit !found }' "$2"
}

changes=0

while IFS=$'\t' read -r name color description; do
  if existing="$(lookup "$name" "$current")"; then
    [[ "$existing" != "$color"$'\t'"$description" ]] || continue
    action="update"
  else
    action="create"
  fi

  changes=$((changes + 1))
  printf '%s  %s\n' "$action" "$name"
  if [[ "$dry_run" == false ]]; then
    gh label create "$name" --repo "$repo" --color "$color" --description "$description" --force > /dev/null
  fi
done < "$declared"

if [[ "$prune" == true ]]; then
  while IFS=$'\t' read -r name _; do
    ! lookup "$name" "$declared" > /dev/null || continue

    count="$(gh issue list --repo "$repo" --label "$name" --state all --limit 5000 --json number --jq length)"
    changes=$((changes + 1))
    printf 'delete  %s (%s issues)\n' "$name" "$count"
    if [[ "$dry_run" == false ]]; then
      gh label delete "$name" --repo "$repo" --yes
    fi
  done < "$current"
fi

[[ "$changes" -gt 0 ]] || echo "no changes"
