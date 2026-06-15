package uploader

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/fastpath"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/normalize"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/ringbuffer"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/tetragon"
)

type BatchUploader interface {
	Upload(batch *analyticsv1.UploadBatch) (*analyticsv1.UploadAck, error)
}

type StreamOptions struct {
	AgentID       string
	HostID        string
	TenantID      string
	Scenario      string
	Version       string
	BatchSize     int
	FlushInterval time.Duration
	RawRing       *ringbuffer.Buffer
}

type StreamStats struct {
	Events  int
	Signals int
	Batches int
}

func StreamJSONL(ctx context.Context, r io.Reader, up BatchUploader, opts StreamOptions) (StreamStats, error) {
	if up == nil {
		return StreamStats{}, fmt.Errorf("uploader is nil")
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 128
	}
	if opts.FlushInterval <= 0 {
		opts.FlushInterval = time.Second
	}
	if opts.RawRing == nil {
		opts.RawRing = ringbuffer.New(4096)
	}

	norm := normalize.New(opts.AgentID, opts.HostID, nil)
	fp := fastpath.New()
	lines := scanLines(r)
	ticker := time.NewTicker(opts.FlushInterval)
	defer ticker.Stop()

	batch := newBatch(opts)
	var stats StreamStats
	line := 0
	flush := func() error {
		if len(batch.GetEvents()) == 0 && len(batch.GetSignals()) == 0 {
			return nil
		}
		if _, err := up.Upload(batch); err != nil {
			return err
		}
		stats.Batches++
		batch = newBatch(opts)
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			if err := flush(); err != nil {
				return stats, err
			}
			return stats, ctx.Err()
		case <-ticker.C:
			if err := flush(); err != nil {
				return stats, err
			}
		case scanned, ok := <-lines:
			if !ok {
				if err := flush(); err != nil {
					return stats, err
				}
				return stats, nil
			}
			if scanned.err != nil {
				return stats, scanned.err
			}
			line++
			if len(scanned.data) == 0 {
				continue
			}
			events, signals, err := decodeLine(scanned.data, norm, fp, opts.Scenario, opts.RawRing)
			if err != nil {
				return stats, fmt.Errorf("line %d: %w", line, err)
			}
			batch.Events = append(batch.Events, events...)
			batch.Signals = append(batch.Signals, signals...)
			stats.Events += len(events)
			stats.Signals += len(signals)
			if len(batch.GetEvents())+len(batch.GetSignals()) < opts.BatchSize {
				continue
			}
			if err := flush(); err != nil {
				return stats, err
			}
		}
	}
}

type scannedLine struct {
	data []byte
	err  error
}

func scanLines(r io.Reader) <-chan scannedLine {
	out := make(chan scannedLine)
	go func() {
		defer close(out)
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			out <- scannedLine{data: append([]byte(nil), scanner.Bytes()...)}
		}
		if err := scanner.Err(); err != nil {
			out <- scannedLine{err: err}
		}
	}()
	return out
}

func newBatch(opts StreamOptions) *analyticsv1.UploadBatch {
	tenantID := opts.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	return &analyticsv1.UploadBatch{Agent: &analyticsv1.AgentHello{
		AgentId:  opts.AgentID,
		HostId:   opts.HostID,
		TenantId: tenantID,
		Version:  opts.Version,
	}}
}

func decodeLine(data []byte, norm *normalize.Normalizer, fp *fastpath.Engine, scenario string, rawRing *ringbuffer.Buffer) ([]*eventv1.CanonicalEvent, []*signalv1.Signal, error) {
	if sig, ok := decodeSignal(data); ok {
		if sig.Scenario == "" {
			sig.Scenario = scenario
		}
		return nil, []*signalv1.Signal{sig}, nil
	}
	if ev, ok := decodeEvent(data); ok {
		if ev.Scenario == "" {
			ev.Scenario = scenario
		}
		return []*eventv1.CanonicalEvent{ev}, fp.Process(ev), nil
	}
	if sev, ok := decodeSensorEvent(data); ok {
		sev.RawRef = rawRing.Remember(sev.GetRawRef(), data)
		ev := norm.Normalize(sev)
		ev.Scenario = scenario
		return []*eventv1.CanonicalEvent{ev}, fp.Process(ev), nil
	}
	if sevs, ok := tetragon.ParseLine(data); ok {
		rawRef := rawRing.Put(data)
		events := make([]*eventv1.CanonicalEvent, 0, len(sevs))
		var signals []*signalv1.Signal
		for _, sev := range sevs {
			if sev.GetRawRef() == "" {
				sev.RawRef = rawRef
			} else {
				rawRing.Remember(sev.GetRawRef(), data)
			}
			ev := norm.Normalize(sev)
			ev.Scenario = scenario
			events = append(events, ev)
			signals = append(signals, fp.Process(ev)...)
		}
		return events, signals, nil
	}
	return nil, nil, fmt.Errorf("not Signal, CanonicalEvent, SensorEvent nor Tetragon event")
}
