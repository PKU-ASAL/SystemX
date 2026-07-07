ARG RUNTIME_IMAGE=ubuntu:24.04
FROM ${RUNTIME_IMAGE}
COPY dist/bin/sysarmor-manager /usr/local/bin/sysarmor-manager
EXPOSE 9443
ENTRYPOINT ["/usr/local/bin/sysarmor-manager"]
CMD ["--listen", "0.0.0.0:9443"]
