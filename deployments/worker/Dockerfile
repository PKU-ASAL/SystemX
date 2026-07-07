ARG RUNTIME_IMAGE=alpine:latest
FROM ${RUNTIME_IMAGE}
COPY bin/sysarmor-worker /usr/local/bin/sysarmor-worker
USER nobody:nobody
ENTRYPOINT ["/usr/local/bin/sysarmor-worker"]
