# Used by GoReleaser: the prebuilt static binary is copied in, so this stage
# is packaging only. Defaults suit running behind a proxy or tunnel: token
# auth, listening on all interfaces (the container boundary), state in /data.
# Set QUIRE_AUTH_MODE=passkey and QUIRE_BASE_URL for a human-facing install.
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
COPY quire /usr/local/bin/quire
ENV QUIRE_DATA_DIR=/data \
    QUIRE_ADDR=0.0.0.0:8321 \
    QUIRE_AUTH_MODE=token-only
VOLUME /data
EXPOSE 8321
ENTRYPOINT ["quire"]
CMD ["serve"]
