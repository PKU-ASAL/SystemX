module github.com/sysarmor/sysarmor-next-project

go 1.26.0

require google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af

require github.com/cilium/tetragon/api v1.7.0 // indirect

require (
	github.com/cespare/xxhash/v2 v2.3.0
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f
	github.com/klauspost/compress v1.15.9
	github.com/lib/pq v1.10.9
	github.com/pierrec/lz4/v4 v4.1.15
	github.com/redis/go-redis/v9 v9.18.0
	github.com/segmentio/kafka-go v0.4.51
	go.uber.org/atomic v1.11.0
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260209200024-4cfbd4190f57 // indirect
	google.golang.org/grpc v1.80.0 // indirect
)
