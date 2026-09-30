#!/usr/bin/env bash
set -euo pipefail

# GitHub's ubuntu runners ship ~14GB of preinstalled toolchains the e2e jobs
# never use. The e2e Docker image plus its build cache plus per-test Playwright
# trace recording can exhaust the remaining headroom — the burn-in lane
# (~190 traced test executions) died with ENOSPC. Reclaim the space up front.
# CI-only: refuse to run anywhere else so it can never eat a real machine.
if [[ "${GITHUB_ACTIONS:-}" != "true" ]]; then
  echo "error: free-ci-disk.sh is destructive and only runs on GitHub Actions runners" >&2
  exit 1
fi

df -h / | tail -1
sudo rm -rf /usr/local/lib/android /usr/share/dotnet /opt/ghc /usr/local/.ghcup /opt/hostedtoolcache/CodeQL
sudo docker image prune --all --force >/dev/null
df -h / | tail -1
