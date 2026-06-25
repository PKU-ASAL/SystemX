package tamper

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
)

const SignalName = "sensor_tamper_or_blindness"

type Detector struct {
	lastID string
}

type Options struct {
	MaxRestarts        uint64
	MaxParseErrors     uint64
	MaxDroppedEvents   uint64
	NoEventGracePeriod time.Duration
}

func DefaultOptions() Options {
	return Options{
		MaxRestarts:        0,
		MaxParseErrors:     0,
		MaxDroppedEvents:   0,
		NoEventGracePeriod: time.Minute,
	}
}

func (d *Detector) Evaluate(health agenthealth.AgentHealth, now time.Time, opts Options) *signalv1.Signal {
	reason := Reason(health, now, opts)
	if reason == "" {
		return nil
	}
	id := signalID(health, reason, now)
	if id == d.lastID {
		return nil
	}
	d.lastID = id
	return Signal(health, reason, id)
}

func Reason(health agenthealth.AgentHealth, now time.Time, opts Options) string {
	sensor := health.Sensor
	switch {
	case !sensor.PolicyLoaded:
		return "policy_not_loaded"
	case !sensor.Running:
		return "sensor_not_running"
	case sensor.LastError != "":
		return "sensor_error:" + sensor.LastError
	case opts.MaxRestarts > 0 && sensor.RestartCount > opts.MaxRestarts:
		return fmt.Sprintf("restart_count_exceeded:%d>%d", sensor.RestartCount, opts.MaxRestarts)
	case opts.MaxParseErrors > 0 && sensor.ParseErrors > opts.MaxParseErrors:
		return fmt.Sprintf("parse_errors_exceeded:%d>%d", sensor.ParseErrors, opts.MaxParseErrors)
	case opts.MaxDroppedEvents > 0 && sensor.EventsDropped > opts.MaxDroppedEvents:
		return fmt.Sprintf("events_dropped_exceeded:%d>%d", sensor.EventsDropped, opts.MaxDroppedEvents)
	case opts.NoEventGracePeriod > 0 && sensor.Running && sensor.LastEventAt.IsZero() && health.UptimeSeconds >= int64(opts.NoEventGracePeriod.Seconds()):
		return "event_stream_blind:no_events_seen"
	case opts.NoEventGracePeriod > 0 && sensor.Running && !sensor.LastEventAt.IsZero() && now.Sub(sensor.LastEventAt) > opts.NoEventGracePeriod:
		return "event_stream_blind:" + now.Sub(sensor.LastEventAt).String()
	default:
		return ""
	}
}

func Signal(health agenthealth.AgentHealth, reason, id string) *signalv1.Signal {
	if id == "" {
		id = signalID(health, reason, health.ObservedAt)
	}
	entities := []*signalv1.EntityRef{
		{Kind: "agent", Key: "agent:" + health.AgentID, Role: "subject"},
		{Kind: "host", Key: "host:" + health.HostID, Role: "host"},
		{Kind: "sensor", Key: "sensor:" + firstNonEmpty(health.Sensor.Backend, "unknown"), Role: "object"},
	}
	if scopeKey := scopeEntityKey(health.Scope); scopeKey != "" {
		entities = append(entities, &signalv1.EntityRef{Kind: "scope", Key: scopeKey, Role: "scope"})
	}
	summary := fmt.Sprintf("sensor tamper/blindness: %s", reason)
	if scopeSummary := scopeSummary(health.Scope); scopeSummary != "" {
		summary += "; scope=" + scopeSummary
	}
	summary += fmt.Sprintf("; restarts=%d parse_errors=%d dropped_events=%d events_seen=%d",
		health.Sensor.RestartCount,
		health.Sensor.ParseErrors,
		health.Sensor.EventsDropped,
		health.Sensor.EventsSeen,
	)
	if health.Sensor.LastExitReason != "" {
		summary += "; last_exit=" + health.Sensor.LastExitReason
	}
	return &signalv1.Signal{
		Id:           id,
		Name:         SignalName,
		Where:        signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT,
		BaseRisk:     90,
		LocalRarity:  1,
		GlobalRarity: 1,
		LineageId:    "agent:" + health.AgentID,
		Entities:     entities,
		Terminal:     true,
		Evidence: &signalv1.EvidenceBundle{
			Id:       "evb-" + id,
			Entities: entities,
			Summary:  summary,
		},
		Labels: map[string]string{"signal_class": "agent-health"},
	}
}

func signalID(health agenthealth.AgentHealth, reason string, now time.Time) string {
	bucket := now.UTC().Unix() / int64((5 * time.Minute).Seconds())
	base := strings.Join([]string{health.TenantID, health.AgentID, health.HostID, scopeSummary(health.Scope), health.Sensor.Backend, reason, fmt.Sprint(bucket)}, "|")
	sum := sha256.Sum256([]byte(base))
	return "sig-tamper-" + hex.EncodeToString(sum[:8])
}

func scopeEntityKey(scope agenthealth.RuntimeScope) string {
	summary := scopeSummary(scope)
	if summary == "" {
		return ""
	}
	return "scope:" + summary
}

func scopeSummary(scope agenthealth.RuntimeScope) string {
	scopeType := strings.TrimSpace(scope.Type)
	if scopeType == "" {
		return ""
	}
	selector := strings.TrimSpace(scope.Selector)
	if selector == "" {
		return scopeType
	}
	return scopeType + ":" + selector
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
