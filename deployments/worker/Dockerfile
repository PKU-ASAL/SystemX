FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/sysarmor-worker ./cmd/sysarmor-worker

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/sysarmor-worker /usr/local/bin/sysarmor-worker
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/sysarmor-worker"]
