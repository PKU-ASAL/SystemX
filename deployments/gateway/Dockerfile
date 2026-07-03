FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/sysarmor-gateway ./cmd/sysarmor-gateway

FROM ubuntu:22.04
COPY --from=build /out/sysarmor-gateway /usr/local/bin/sysarmor-gateway
EXPOSE 9444 9445
ENTRYPOINT ["/usr/local/bin/sysarmor-gateway"]
CMD ["--listen", "0.0.0.0:9444", "--health-listen", "0.0.0.0:9445"]
