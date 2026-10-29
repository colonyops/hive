# An ssh host with hive, tmux, and git for the remote tests and test/sandbox.
# The build context is the repository root; run.sh builds the binary first.
FROM debian:bookworm-slim@sha256:7ff6ba31527b9e336162fa061746c366c4b2032f4d2cff2078bea43f78efe3f8

RUN apt-get update && apt-get install -y --no-install-recommends \
    openssh-server \
    tmux \
    git \
    procps \
    iproute2 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    # sshd refuses a locked account when UsePAM is off; PasswordAuthentication
    # is off, so the empty password never authenticates.
    && useradd -m -s /bin/bash hive && passwd -d hive \
    && mkdir -p /run/sshd /etc/ssh/keys

ARG TARGETARCH
COPY --chmod=755 dist/hive_linux_${TARGETARCH}_*/hive /usr/local/bin/hive
COPY test/remote/sshd_config /etc/ssh/sshd_config
COPY --chmod=755 test/remote/fake-claude /usr/local/bin/claude
COPY internal/domain/terminal/assess/testdata/claude/ /usr/local/share/fake-claude/
COPY --chmod=755 test/remote/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY --chown=hive:hive test/remote/config.yaml /home/hive/.config/hive/config.yaml

# --no-hardlinks: a later clone of repo.git fails with "hardlink different from
# source" when objects in an image layer already have a second link.
USER hive
RUN git config --global user.name fixture \
    && git config --global user.email fixture@example.invalid \
    && git config --global init.defaultBranch main \
    # Debian's ~/.profile adds this directory to PATH in a login shell only,
    # so a test can tell the login PATH from the PATH of a bare ssh command.
    && mkdir -p /home/hive/.local/bin \
    && git init -q --bare /home/hive/repo.git \
    && git clone -q /home/hive/repo.git /tmp/seed \
    && git -C /tmp/seed commit -q --allow-empty -m seed \
    && git -C /tmp/seed push -q origin main \
    && git clone -q --bare --no-local /home/hive/repo.git /home/hive/windows.git \
    && rm -rf /tmp/seed \
    && git clone -q --no-hardlinks /home/hive/repo.git /home/hive/work
USER root

EXPOSE 22
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
