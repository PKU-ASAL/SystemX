ARG RUNTIME_IMAGE=ubuntu:24.04
FROM ${RUNTIME_IMAGE}
COPY bin/sysarmor-gateway /usr/local/bin/sysarmor-gateway
EXPOSE 9444 9445
ENTRYPOINT ["/usr/local/bin/sysarmor-gateway"]
CMD ["--listen", "0.0.0.0:9444", "--health-listen", "0.0.0.0:9445"]
