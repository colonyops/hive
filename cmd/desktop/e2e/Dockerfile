# Browser image version matches cmd/desktop/e2e/package-lock.json's @playwright/test.
# Index digests retain the upstream multi-architecture manifests.
FROM golang:1.27-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195 AS go

FROM mcr.microsoft.com/playwright:v1.62.1-noble@sha256:dcc5531e97840b9b5e794f2814476b21571c5124a3fca2267d73041f56e7580e
COPY --from=go /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:${PATH} \
    CI=1

# The server-build action runtime uses git and tmux; bash, curl, and coreutils
# are used by the isolated e2e harness. The Playwright base image already has
# the matching browser system dependencies installed.
RUN apt-get update && apt-get install -y --no-install-recommends \
      git tmux bash curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/desktop/frontend/package.json cmd/desktop/frontend/package-lock.json cmd/desktop/frontend/.npmrc ./cmd/desktop/frontend/
RUN cd cmd/desktop/frontend && npm ci
COPY cmd/desktop/e2e/package.json cmd/desktop/e2e/package-lock.json ./cmd/desktop/e2e/
RUN cd cmd/desktop/e2e && npm ci
COPY . .

# Build the frontend and the headless server at image-build time rather than
# inside Playwright's webServer launcher: compile failures surface here with
# clean attribution instead of as a mysterious "webServer was not able to
# start", and the webServer timeout then covers server startup alone.
RUN cd cmd/desktop/frontend && npm run build
RUN CGO_ENABLED=0 go build -tags server -o cmd/desktop/bin/hive-desktop-server ./cmd/desktop

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
