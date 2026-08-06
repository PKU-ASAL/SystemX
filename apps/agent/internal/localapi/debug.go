package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

func (h *Handler) DebugProfile(ctx context.Context, req *controlplanev1.DebugProfileRequest) (*controlplanev1.DebugProfileResponse, error) {
	if err := h.validate(req.GetContext()); err != nil {
		return nil, err
	}
	profileType, seconds, label, err := debugProfileOptions(req)
	if err != nil {
		return nil, err
	}
	if !h.profileMu.TryLock() {
		return nil, fmt.Errorf("debug profile already running")
	}
	defer h.profileMu.Unlock()
	started := time.Now().UTC()
	profile, err := captureDebugProfile(ctx, profileType, seconds, label, started)
	if err != nil {
		return nil, err
	}
	return &controlplanev1.DebugProfileResponse{
		ProfileType: profileType, Seconds: seconds, StartedAt: started.Format(time.RFC3339Nano),
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), Profile: profile, Label: label,
	}, nil
}

func debugProfileOptions(req *controlplanev1.DebugProfileRequest) (string, uint32, string, error) {
	profileType := strings.TrimSpace(req.GetProfileType())
	if profileType == "" {
		profileType = "cpu"
	}
	switch profileType {
	case "cpu", "heap", "allocs", "goroutine", "threadcreate", "block", "mutex", "runtime":
	default:
		return "", 0, "", fmt.Errorf("unsupported debug profile type %q", profileType)
	}
	seconds := req.GetSeconds()
	if seconds == 0 {
		seconds = 10
	}
	if seconds > 300 {
		return "", 0, "", fmt.Errorf("debug profile seconds must be <= 300")
	}
	return profileType, seconds, strings.TrimSpace(req.GetLabel()), nil
}

func captureDebugProfile(ctx context.Context, profileType string, seconds uint32, label string, started time.Time) ([]byte, error) {
	if profileType == "cpu" {
		return captureCPUProfile(ctx, time.Duration(seconds)*time.Second)
	}
	var buf bytes.Buffer
	if profileType == "runtime" {
		if err := json.NewEncoder(&buf).Encode(runtimeStatsPayload(started, label)); err != nil {
			return nil, fmt.Errorf("encode runtime stats: %w", err)
		}
		return buf.Bytes(), nil
	}
	runtime.GC()
	profile := pprof.Lookup(profileType)
	if profile == nil {
		return nil, fmt.Errorf("profile %q unavailable", profileType)
	}
	if err := profile.WriteTo(&buf, 0); err != nil {
		return nil, fmt.Errorf("write %s profile: %w", profileType, err)
	}
	return buf.Bytes(), nil
}

func captureCPUProfile(ctx context.Context, duration time.Duration) ([]byte, error) {
	var buf bytes.Buffer
	if err := pprof.StartCPUProfile(&buf); err != nil {
		return nil, fmt.Errorf("start cpu profile: %w", err)
	}
	timer := time.NewTimer(duration)
	select {
	case <-ctx.Done():
		timer.Stop()
		pprof.StopCPUProfile()
		return nil, ctx.Err()
	case <-timer.C:
		pprof.StopCPUProfile()
		return buf.Bytes(), nil
	}
}

func runtimeStatsPayload(observedAt time.Time, label string) map[string]any {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return map[string]any{
		"observed_at":         observedAt.Format(time.RFC3339Nano),
		"label":               label,
		"go_version":          runtime.Version(),
		"goos":                runtime.GOOS,
		"goarch":              runtime.GOARCH,
		"gomaxprocs":          runtime.GOMAXPROCS(0),
		"goroutines":          runtime.NumGoroutine(),
		"cgo_calls":           runtime.NumCgoCall(),
		"heap_alloc_bytes":    mem.HeapAlloc,
		"heap_sys_bytes":      mem.HeapSys,
		"heap_idle_bytes":     mem.HeapIdle,
		"heap_inuse_bytes":    mem.HeapInuse,
		"heap_released_bytes": mem.HeapReleased,
		"heap_objects":        mem.HeapObjects,
		"stack_inuse_bytes":   mem.StackInuse,
		"stack_sys_bytes":     mem.StackSys,
		"alloc_bytes_total":   mem.TotalAlloc,
		"mallocs_total":       mem.Mallocs,
		"frees_total":         mem.Frees,
		"gc_count":            mem.NumGC,
		"gc_pause_ns_total":   mem.PauseTotalNs,
		"last_gc_unix_ns":     mem.LastGC,
		"next_gc_bytes":       mem.NextGC,
		"gc_cpu_fraction":     mem.GCCPUFraction,
	}
}
