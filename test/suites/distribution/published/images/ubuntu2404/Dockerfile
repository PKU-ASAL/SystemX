FROM ubuntu:24.04

ARG SYSARMOR_INSTALL_URL
ARG SYSARMOR_TETRAGON_URL
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends bash ca-certificates curl jq nodejs procps sqlite3 util-linux \
    && rm -rf /var/lib/apt/lists/*
RUN test -n "$SYSARMOR_INSTALL_URL" \
    && base_url="${SYSARMOR_INSTALL_URL%/*}" \
    && curl -fsSL "$SYSARMOR_INSTALL_URL" \
       | SYSARMOR_RELEASE_BASE_URL="$base_url" SYSARMOR_TETRAGON_URL="$SYSARMOR_TETRAGON_URL" \
         bash -s -- --profile linux-container

COPY fixtures /opt/sysarmor-release-test

ENTRYPOINT ["/usr/local/bin/sysarmor-container-entrypoint"]
CMD ["node", "/opt/sysarmor-release-test/web-app/server.js"]
