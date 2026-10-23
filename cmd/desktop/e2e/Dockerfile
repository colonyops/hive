# Browser image version matches cmd/desktop/e2e/package-lock.json's @playwright/test.
# Index digests retain the upstream multi-architecture manifests.
FROM mcr.microsoft.com/playwright:v1.62.1-noble@sha256:dcc5531e97840b9b5e794f2814476b21571c5124a3fca2267d73041f56e7580e
ENV CI=1

# The server-build action runtime uses git and tmux; bash, curl, and coreutils
# are used by the isolated e2e harness. The Playwright base image already has
# the browser system dependencies and all of these but tmux. Naming the others
# here upgrades them, and the image's Azure mirror has served those downloads
# at 45 kB/s.
RUN apt-get update && apt-get install -y --no-install-recommends tmux \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
COPY cmd/desktop/e2e/package.json cmd/desktop/e2e/package-lock.json ./cmd/desktop/e2e/
RUN cd cmd/desktop/e2e && npm ci
COPY . .

# Playwright strips types without checking them, so a broken import would
# otherwise surface mid-suite.
RUN cd cmd/desktop/e2e && npx tsc --noEmit

# run-docker.sh builds the server before the image, so compile failures surface
# there with clean attribution, and the webServer timeout covers server startup
# alone.
COPY cmd/desktop/e2e/.build/hive-desktop-server cmd/desktop/bin/hive-desktop-server

# Entrypoint validates the per-run harness marker minted by run-docker.sh,
# then execs its arguments from cmd/desktop/e2e. The default CMD is the full
# Playwright suite; run-docker.sh appends its own arguments after the image
# name, which replace the CMD. Docker-entrypoint convention: a first argument
# that is a flag is treated as Playwright options, so
# `run-docker.sh --grep foo --repeat-each 5` runs
# `npx playwright test --grep foo --repeat-each 5`.
COPY <<'ENTRYPOINT_EOF' /usr/local/bin/e2e-entrypoint
#!/usr/bin/env bash
set -euo pipefail
if [[ ! "${HIVE_DESKTOP_E2E_HARNESS:-}" =~ ^[[:xdigit:]]{64}$ ]]; then
  echo 'error: Docker e2e harness marker missing' >&2
  exit 1
fi
cd /workspace/cmd/desktop/e2e
if [[ $# -eq 0 || $1 == -* ]]; then
  set -- npx playwright test "$@"
fi
exec "$@"
ENTRYPOINT_EOF
RUN chmod +x /usr/local/bin/e2e-entrypoint
ENTRYPOINT ["/usr/local/bin/e2e-entrypoint"]
CMD ["npx", "playwright", "test"]
