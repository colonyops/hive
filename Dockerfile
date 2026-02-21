FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    neovim \
    tmux \
    git \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://claude.ai/install.sh | bash

# Create Claude directories and skip onboarding
RUN mkdir -p /root/.claude && \
    echo '{"hasCompletedOnboarding": true}' > /root/.claude.json

# Credentials will be injected at runtime via CLAUDE_CREDENTIALS env var

COPY dist/hive_linux_amd64_v1/hive /usr/local/bin/hive

RUN chmod +x /usr/local/bin/hive

WORKDIR /workspace

# shell and config
ENV SHELL=/bin/bash
ENV PATH="/root/.local/bin:${PATH}"
ENV HIVE_LOG_LEVEL=debug
ENV HIVE_LOG_FILE=/tmp/hive.log
COPY dev/config.dev.yaml /etc/hive/config.yaml
ENV HIVE_CONFIG=/etc/hive/config.yaml

# hv alias
RUN echo "alias hv='tmux new-session -As hive hive'" >> /root/.bashrc

# working repo
RUN git clone https://github.com/colonyops/hive.git

# Write credentials on startup if provided
COPY dev/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["bash"]
