FROM node:22-bookworm AS frontend
WORKDIR /src/cmd/desktop/frontend
COPY cmd/desktop/frontend/package*.json cmd/desktop/frontend/.npmrc ./
RUN npm ci
COPY cmd/desktop/frontend/ ./
COPY cmd/desktop/internal/app/flow/docs/ /src/cmd/desktop/internal/app/flow/docs/
RUN npm run build

FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/cmd/desktop/frontend/dist ./cmd/desktop/frontend/dist
RUN CGO_ENABLED=0 go build -tags server,terminals -o /out/hive-desktop ./cmd/desktop && \
    CGO_ENABLED=0 go build -o /out/hive .

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends openssh-server tmux git bash ca-certificates curl python3 && \
    rm -rf /var/lib/apt/lists/* && useradd -m -s /bin/bash hive && passwd -d hive && mkdir /run/sshd
COPY --from=build /out/ /usr/local/bin/
COPY cmd/desktop/remote/entrypoint.sh cmd/desktop/remote/start-hive.sh /usr/local/bin/
COPY cmd/desktop/remote/sshd_config /etc/ssh/sshd_config
COPY cmd/desktop/remote/hive.yaml /etc/hive-fixture.yaml
ENV HIVE_CONFIG=/home/hive/.config/hive/config.yaml \
    HIVE_DATA_DIR=/home/hive/.local/share/hive \
    HIVE_DESKTOP_HIVE_DATA_DIR=/home/hive/.local/share/hive \
    HIVE_DESKTOP_DATA_DIR=/home/hive/.local/share/hive-desktop \
    HIVE_DESKTOP_CONFIG_DIR=/home/hive/.config/hive/desktop \
    HIVE_DESKTOP_HTTP_PORT=19001 \
    HIVE_DESKTOP_CONNECTION_FILE=/home/hive/connection.json \
    WAILS_SERVER_HOST=127.0.0.1 \
    WAILS_SERVER_PORT=8080
EXPOSE 22
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]

FROM mcr.microsoft.com/playwright:v1.62.1-noble AS smoke
RUN apt-get update && apt-get install -y --no-install-recommends openssh-client curl && rm -rf /var/lib/apt/lists/*
WORKDIR /smoke
COPY cmd/desktop/e2e/package*.json ./
RUN npm ci
COPY --from=build /out/ /usr/local/bin/
COPY cmd/desktop/remote/smoke.* ./
ENTRYPOINT ["/smoke/smoke.sh"]
