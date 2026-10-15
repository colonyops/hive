# Browser image version matches cmd/drive-server/e2e/package-lock.json's @playwright/test.
# The digests are the ones cmd/desktop/e2e/Dockerfile pins.
FROM golang:1.27-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195 AS go

FROM mcr.microsoft.com/playwright:v1.62.1-noble@sha256:dcc5531e97840b9b5e794f2814476b21571c5124a3fca2267d73041f56e7580e
COPY --from=go /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:${PATH} \
    CI=1

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY package.json package-lock.json .npmrc ./
COPY web/packages/ui/package.json ./web/packages/ui/
COPY cmd/desktop/frontend/package.json ./cmd/desktop/frontend/
COPY cmd/drive-server/frontend/package.json ./cmd/drive-server/frontend/
RUN npm ci
COPY cmd/drive-server/e2e/package.json cmd/drive-server/e2e/package-lock.json ./cmd/drive-server/e2e/
RUN cd cmd/drive-server/e2e && npm ci
COPY . .

# Build at image-build time so a compile failure is attributed to the build,
# and the webServer timeout covers server startup alone.
RUN npm run build -w drive-server-frontend
RUN CGO_ENABLED=0 go build -o cmd/drive-server/bin/hive-drive-server ./cmd/drive-server

# The entrypoint refuses to run without the per-run marker minted by
# run-docker.sh, so a bare `docker run` cannot start the suite by accident.
COPY <<'ENTRYPOINT_EOF' /usr/local/bin/e2e-entrypoint
#!/usr/bin/env bash
set -euo pipefail
if [[ ! "${HIVE_DRIVE_E2E_HARNESS:-}" =~ ^[[:xdigit:]]{64}$ ]]; then
  echo 'error: Docker e2e harness marker missing' >&2
  exit 1
fi
cd /workspace/cmd/drive-server/e2e
if [[ $# -eq 0 || $1 == -* ]]; then
  set -- npx playwright test "$@"
fi
exec "$@"
ENTRYPOINT_EOF
RUN chmod +x /usr/local/bin/e2e-entrypoint
ENTRYPOINT ["/usr/local/bin/e2e-entrypoint"]
CMD ["npx", "playwright", "test"]
