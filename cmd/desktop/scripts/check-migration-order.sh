#!/usr/bin/env bash
set -euo pipefail

root=$(git rev-parse --show-toplevel)
cd "$root"

migration_dir=cmd/desktop/internal/app/data/queries/migrations
current=$(find "$migration_dir" -maxdepth 1 -type f -name '*.up.sql' -print | LC_ALL=C sort)
if [[ -z "$current" ]]; then
  echo "no migrations found under $migration_dir" >&2
  exit 1
fi

expected=1
while IFS= read -r path; do
  name=${path##*/}
  if [[ ! $name =~ ^([0-9]{4})_[a-z0-9_]+\.up\.sql$ ]]; then
    echo "invalid migration filename: $path" >&2
    exit 1
  fi
  version=$((10#${BASH_REMATCH[1]}))
  if [[ $version -ne $expected ]]; then
    printf 'migration order has version %04d at %s; expected %04d\n' "$version" "$path" "$expected" >&2
    exit 1
  fi
  expected=$((expected + 1))
done <<< "$current"

base_ref=${MIGRATION_BASE_REF:-}
if [[ -z $base_ref ]]; then
  base_ref=$(git describe --tags --match 'v[0-9]*' --match 'desktop-v*' --abbrev=0 HEAD 2>/dev/null || true)
fi
if [[ -z $base_ref ]]; then
  echo "migration order valid; no prior release tag found"
  exit 0
fi

base=$(git ls-tree -r --name-only "$base_ref" -- "$migration_dir" | grep '\.up\.sql$' | LC_ALL=C sort || true)
base_dir=$migration_dir

if [[ -z $base ]]; then
  echo "migration order valid against $base_ref; it has no desktop migrations"
  exit 0
fi

base_max=0
while IFS= read -r base_path; do
  name=${base_path##*/}
  if [[ ! $name =~ ^([0-9]{4})_[a-z0-9_]+\.up\.sql$ ]]; then
    echo "invalid migration filename in $base_ref: $base_path" >&2
    exit 1
  fi
  version=$((10#${BASH_REMATCH[1]}))
  if [[ $version -gt $base_max ]]; then
    base_max=$version
  fi
  path="$migration_dir/$name"
  if [[ ! -f $path ]]; then
    echo "released migration was removed: $path (from $base_ref:$base_path)" >&2
    exit 1
  fi
  if ! cmp -s <(git show "$base_ref:$base_path") "$path"; then
    echo "released migration was modified: $path (from $base_ref:$base_path)" >&2
    exit 1
  fi
done <<< "$base"

while IFS= read -r path; do
  name=${path##*/}
  base_path="$base_dir/$name"
  if git cat-file -e "$base_ref:$base_path" 2>/dev/null; then
    continue
  fi
  version=$((10#${name%%_*}))
  if [[ $version -le $base_max ]]; then
    printf 'new migration %s reuses released version %04d from %s\n' "$path" "$version" "$base_ref" >&2
    exit 1
  fi
done <<< "$current"

printf 'migration order valid: %04d migrations, released history unchanged since %s\n' "$((expected - 1))" "$base_ref"
