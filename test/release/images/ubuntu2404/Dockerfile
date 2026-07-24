FROM ubuntu:24.04

ARG SYSARMOR_INSTALL_URL
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends bash ca-certificates curl procps sqlite3 util-linux \
    && rm -rf /var/lib/apt/lists/*
RUN test -n "$SYSARMOR_INSTALL_URL" \
    && base_url="${SYSARMOR_INSTALL_URL%/*}" \
    && curl -fsSL "$SYSARMOR_INSTALL_URL" \
       | SYSARMOR_RELEASE_BASE_URL="$base_url" bash -s -- --profile linux-container

ENTRYPOINT ["/usr/local/bin/sysarmor-container-entrypoint"]
CMD ["sleep", "infinity"]
