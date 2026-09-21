FROM debian:bookworm-slim

# Everything above the final hive COPY is stable and stays cached across
# rebuilds. Keep the binary copy last so only that layer is invalidated.

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    neovim \
    tmux \
    git \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Stub agent binaries so `hive init` agent detection finds them
RUN for agent in pi codex copilot cursor amp opencode; do \
    printf '#!/bin/sh\n' > /usr/local/bin/$agent && \
    chmod +x /usr/local/bin/$agent; \
done

# Non-root user
RUN useradd --create-home --shell /bin/bash --uid 1000 hive && \
    mkdir -p /workspace && chown hive:hive /workspace

COPY dev/config.dev.yaml /etc/hive/config.yaml

# Write credentials on startup if provided
COPY --chmod=755 dev/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

USER hive
WORKDIR /workspace

RUN curl -fsSL https://claude.ai/install.sh | bash

# Create Claude directories and skip onboarding.
# Credentials will be injected at runtime via CLAUDE_CREDENTIALS env var.
RUN mkdir -p /home/hive/.claude && \
    echo '{"hasCompletedOnboarding": true}' > /home/hive/.claude.json

# shell and config
ENV SHELL=/bin/bash
ENV PATH="/home/hive/.local/bin:${PATH}"
ENV HIVE_LOG_LEVEL=debug
ENV HIVE_LOG_FILE=/tmp/hive.log
ENV HIVE_CONFIG=/etc/hive/config.yaml

# hv alias
RUN echo "alias hv='tmux new-session -As hive hive'" >> /home/hive/.bashrc

# working repo
RUN git clone https://github.com/colonyops/hive.git

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["bash"]

# Built hive binary (supports multiple TARGETARCH values). Must stay last.
ARG TARGETARCH=amd64
COPY --chmod=755 dist/hive_linux_${TARGETARCH}_*/hive /usr/local/bin/hive
