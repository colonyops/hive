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

COPY dist/hive_linux_amd64_v1/hive /usr/local/bin/hive
RUN chmod +x /usr/local/bin/hive

WORKDIR /workspace

ENV SHELL=/bin/bash
ENV PATH="/root/.local/bin:${PATH}"
ENV HIVE_LOG_LEVEL=debug
ENV HIVE_LOG_FILE=/tmp/hive.log

# hv alias
RUN echo "alias hv='tmux new-session -As hive hive'" >> /root/.bashrc

# SETUP=full pre-configures a workspace and clones the hive repo.
# SETUP=clean leaves the container vanilla for install wizard testing.
ARG SETUP=full
ENV CONTAINER_SETUP=$SETUP

RUN if [ "$SETUP" = "full" ]; then \
    mkdir -p /root/.config/hive && \
    printf 'workspaces:\n  - /workspace\n' > /root/.config/hive/config.yaml && \
    git clone https://github.com/colonyops/hive.git /workspace/hive; \
fi

COPY dev/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["bash"]
