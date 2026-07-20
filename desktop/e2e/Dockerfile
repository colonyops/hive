# Browser image version matches desktop/e2e/package-lock.json's @playwright/test.
# Index digests retain the upstream multi-architecture manifests.
FROM golang:1.26-bookworm@sha256:1ecb7edf62a0408027bd5729dfd6b1b8766e578e8df93995b225dfd0944eb651 AS go

FROM mcr.microsoft.com/playwright:v1.61.1-noble@sha256:5b8f294aff9041b7191c34a4bab3ac270157a28774d4b0660e9743297b697e48
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
COPY desktop/frontend/package.json desktop/frontend/package-lock.json desktop/frontend/.npmrc ./desktop/frontend/
RUN cd desktop/frontend && npm ci
COPY desktop/e2e/package.json desktop/e2e/package-lock.json ./desktop/e2e/
RUN cd desktop/e2e && npm ci
COPY . .

CMD ["bash", "-ec", "if [[ ! \"${HIVE_DESKTOP_E2E_HARNESS:-}\" =~ ^[[:xdigit:]]{64}$ ]]; then echo 'error: Docker e2e harness marker missing' >&2; exit 1; fi; cd desktop/e2e && npx playwright test"]
