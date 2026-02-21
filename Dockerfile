FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    tmux \
    git \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://claude.ai/install.sh | bash

COPY dist/hive_linux_amd64_v1/hive /usr/local/bin/hive

RUN chmod +x /usr/local/bin/hive

WORKDIR /workspace

RUN git clone https://github.com/colonyops/hive.git .

CMD ["bash"]
