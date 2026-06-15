package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	agentconfig "github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/daemon"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/uploader"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Println(version)
			return
		case "run":
			if err := runDaemonCommand(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "sysarmor-agent run: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	manager := flag.String("manager", "127.0.0.1:9443", "sysarmor-manager address")
	transport := flag.String("transport", "http", "upload transport: http or grpc")
	agentID := flag.String("agent-id", "agent-dev", "agent identifier")
	hostID := flag.String("host-id", "host-dev", "host identifier")
	scenario := flag.String("scenario", "", "scenario label for replayed events")
	input := flag.String("input-jsonl", "", "upload CanonicalEvent/Signal protojson lines from this file")
	stream := flag.String("stream-jsonl", "", "stream SensorEvent/Tetragon JSONL from this file, or '-' for stdin")
	batchSize := flag.Int("batch-size", 128, "stream upload batch size")
	flushInterval := flag.Duration("flush-interval", time.Second, "stream upload flush interval")
	flag.Parse()

	if flag.NArg() > 0 && flag.Arg(0) == "version" {
		fmt.Println(version)
		return
	}

	if *input != "" {
		if err := uploadJSONL(*manager, *transport, *agentID, *hostID, *scenario, *input); err != nil {
			fmt.Fprintf(os.Stderr, "sysarmor-agent: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *stream != "" {
		stats, err := streamJSONL(*manager, *transport, *agentID, *hostID, *scenario, *stream, *batchSize, *flushInterval)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sysarmor-agent: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "sysarmor-agent stream complete: events=%d signals=%d batches=%d\n", stats.Events, stats.Signals, stats.Batches)
		return
	}

	fmt.Fprintf(os.Stderr, "sysarmor-agent skeleton: agent_id=%s host_id=%s manager=%s\n", *agentID, *hostID, *manager)
}

func runDaemonCommand(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "/etc/sysarmor/agent.yaml", "agent config path")
	dryRun := fs.Bool("dry-run", false, "validate config and exit")
	once := fs.Bool("once", false, "run until the first daemon event or health tick and exit")
	drainOnce := fs.Bool("drain-once", false, "attempt one oldest-first spool upload drain after receiving an event")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := agentconfig.LoadFile(*configPath)
	if err != nil {
		return err
	}
	if *dryRun {
		fmt.Fprintf(os.Stdout, "config ok: agent=%s host=%s tenant=%s manager=%s sensor=%s/%s\n",
			cfg.Agent.ID, cfg.Agent.HostID, cfg.Agent.TenantID, cfg.Manager.Address, cfg.Sensor.Backend, cfg.Sensor.Mode)
		return nil
	}
	runner, err := daemon.New(cfg)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runner.Run(ctx, daemon.Options{Once: *once, DrainOnce: *drainOnce, Out: os.Stdout})
}

func uploadJSONL(manager, transport, agentID, hostID, scenario, input string) error {
	f, err := os.Open(input)
	if err != nil {
		return err
	}
	defer f.Close()
	batch, err := uploader.ReadProtoJSONL(f, agentID, hostID, scenario)
	if err != nil {
		return err
	}
	batch.Agent = &analyticsv1.AgentHello{
		AgentId: agentID,
		HostId:  hostID,
		Version: version,
	}
	up, err := newUploader(manager, transport)
	if err != nil {
		return err
	}
	return up.Upload(batch)
}

func streamJSONL(manager, transport, agentID, hostID, scenario, input string, batchSize int, flushInterval time.Duration) (uploader.StreamStats, error) {
	r := os.Stdin
	if input != "-" {
		f, err := os.Open(input)
		if err != nil {
			return uploader.StreamStats{}, err
		}
		defer f.Close()
		r = f
	}
	up, err := newUploader(manager, transport)
	if err != nil {
		return uploader.StreamStats{}, err
	}
	return uploader.StreamJSONL(context.Background(), r, up, uploader.StreamOptions{
		AgentID:       agentID,
		HostID:        hostID,
		Scenario:      scenario,
		Version:       version,
		BatchSize:     batchSize,
		FlushInterval: flushInterval,
	})
}

func newUploader(manager, transport string) (uploader.BatchUploader, error) {
	switch transport {
	case "http":
		return uploader.NewHTTPUploader(manager), nil
	case "grpc":
		return uploader.NewGRPCUploader(manager), nil
	default:
		return nil, fmt.Errorf("unknown transport %q", transport)
	}
}
