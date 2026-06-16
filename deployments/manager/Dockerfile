FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/sysarmor-manager ./cmd/sysarmor-manager

FROM ubuntu:22.04
COPY --from=build /out/sysarmor-manager /usr/local/bin/sysarmor-manager
EXPOSE 9443 9444
ENTRYPOINT ["sysarmor-manager", "--listen", "0.0.0.0:9443", "--grpc-listen", "0.0.0.0:9444"]
