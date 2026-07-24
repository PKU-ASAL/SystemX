package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/localstore"
)

type options struct {
	mode       string
	stateDir   string
	marker     string
	signalRule string
}

func main() {
	opts := parseFlags()
	if err := inspect(context.Background(), opts); err != nil {
		fmt.Fprintf(os.Stderr, "[release-inspect][ERROR] %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() options {
	var opts options
	flag.StringVar(&opts.mode, "mode", "positive", "positive or absent")
	flag.StringVar(&opts.stateDir, "state-dir", "", "copied standalone state directory")
	flag.StringVar(&opts.marker, "marker", "", "unique event argv marker")
	flag.StringVar(&opts.signalRule, "signal-rule", "web_runtime_spawns_shell", "required signal rule")
	flag.Parse()
	return opts
}

func inspect(ctx context.Context, opts options) error {
	if opts.stateDir == "" || opts.marker == "" {
		return fmt.Errorf("state-dir and marker are required")
	}
	store, err := localstore.Open(ctx, localstore.Options{RootDir: opts.stateDir})
	if err != nil {
		return fmt.Errorf("open copied local state: %w", err)
	}
	defer store.Close()

	events, err := store.QueryEvents(ctx, localstore.EventQuery{Limit: 1000})
	if err != nil {
		return fmt.Errorf("query events: %w", err)
	}
	var matched *eventv1.CanonicalEvent
	for _, frame := range events {
		if eventHasMarker(frame.GetEvent(), opts.marker) {
			matched = frame.GetEvent()
			break
		}
	}
	if opts.mode == "absent" {
		if matched != nil {
			return fmt.Errorf("namespace/self captured marker %q: %s", opts.marker, eventSummary(matched))
		}
		return nil
	}
	if opts.mode != "positive" {
		return fmt.Errorf("unsupported mode %q", opts.mode)
	}
	if matched == nil {
		return fmt.Errorf("event marker %q not found in %d recent events", opts.marker, len(events))
	}
	return requireSignal(ctx, store, opts.signalRule, matched.GetId())
}

func eventSummary(event *eventv1.CanonicalEvent) string {
	proc := event.GetSubjectProc()
	scope := event.GetScope()
	return fmt.Sprintf("binary=%q argv=%q container_id=%q scope=%s/%s",
		proc.GetBinary(), proc.GetArgv(), event.GetContainerId(), scope.GetType(), scope.GetSelector())
}

func requireSignal(ctx context.Context, store *localstore.Store, rule, eventID string) error {
	signals, err := store.QuerySignals(ctx, localstore.SignalQuery{RuleID: rule, Severity: "high", Limit: 1000})
	if err != nil {
		return fmt.Errorf("query signals: %w", err)
	}
	for _, frame := range signals {
		if signalMatchesEvent(frame.GetSignal(), eventID) {
			return nil
		}
	}
	return fmt.Errorf("high signal rule %q does not reference event %q", rule, eventID)
}

func signalMatchesEvent(signal *signalv1.Signal, eventID string) bool {
	if signal == nil || signal.GetSeverity() != "high" || eventID == "" {
		return false
	}
	for _, ref := range signal.GetEventRefs() {
		if ref == eventID {
			return true
		}
	}
	return false
}

func eventHasMarker(event *eventv1.CanonicalEvent, marker string) bool {
	if event == nil || event.GetSubjectProc() == nil {
		return false
	}
	return strings.Contains(strings.Join(event.GetSubjectProc().GetArgv(), " "), marker)
}
