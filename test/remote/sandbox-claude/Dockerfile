# The sandbox server with real Claude Code in place of fake-claude, and
# public repositories to create sessions from. sandbox.sh up --claude builds
# it on top of the remote test image and mounts the token at run time.
FROM hive-sandbox:local

# Pinned to a release at least seven days old, the minimum release age the
# host's package managers enforce.
ARG CLAUDE_CODE_VERSION=2.1.307
RUN apt-get update && apt-get install -y --no-install-recommends nodejs npm curl \
    && rm -rf /var/lib/apt/lists/* \
    && npm install --global --prefix /opt/claude "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}" \
    && rm -f /usr/local/bin/claude
COPY --chmod=755 test/remote/sandbox-claude/claude /usr/local/bin/claude
COPY --chmod=755 test/remote/sandbox-claude/claude-trust /usr/local/bin/claude-trust
COPY --chmod=755 test/remote/sandbox-claude/entrypoint.sh /usr/local/bin/entrypoint-claude.sh
COPY --chown=hive:hive test/remote/sandbox-claude/config.yaml /home/hive/.config/hive/config.yaml
COPY --chown=hive:hive test/remote/sandbox-claude/claude.json /home/hive/.claude.json

USER hive
RUN mkdir -p /home/hive/src \
    && git clone -q https://github.com/hay-kot/appkit.git /home/hive/src/appkit \
    && git clone -q https://github.com/hay-kot/criterio.git /home/hive/src/criterio
USER root

ENTRYPOINT ["/usr/local/bin/entrypoint-claude.sh"]
