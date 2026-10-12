FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

# goreleaser's dockers_v2 puts each platform's binary at
# <os>/<arch>/hive-relay in the build context.
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/hive-relay /hive-relay

# WORKDIR creates /data owned by the image's nonroot user, so a fresh named
# volume mounted there is writable. VOLUME alone would create it as root.
WORKDIR /data
VOLUME /data
ENV RELAY_DATA_DIR=/data

EXPOSE 8080
ENTRYPOINT ["/hive-relay", "serve"]
