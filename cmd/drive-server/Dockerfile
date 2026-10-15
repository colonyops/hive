FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

# goreleaser's dockers_v2 puts each platform's binary at
# <os>/<arch>/hive-drive-server in the build context.
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/hive-drive-server /hive-drive-server

# WORKDIR creates each directory owned by the image's nonroot user, so a fresh
# named volume mounted at /data is writable. VOLUME alone would create it as
# root. The server also needs HIVE_DRIVE_SERVER_BASE_URL and SMTP settings (or
# --dev-log-mail) at start; the image sets no default for them.
WORKDIR /config
WORKDIR /data
VOLUME /data
ENV HIVE_DRIVE_SERVER_DATA_DIR=/data HIVE_DRIVE_SERVER_CONFIG_DIR=/config

EXPOSE 8080
ENTRYPOINT ["/hive-drive-server", "serve"]
