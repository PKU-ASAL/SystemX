package ingestworker

import (
	"context"
	"fmt"
	"strings"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	policyv1 "github.com/sysarmor/sysarmor-next-project/api/proto/policy/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	analyticingest "github.com/sysarmor/sysarmor-next-project/internal/analytics/ingest"
	platformopensearch "github.com/sysarmor/sysarmor-next-project/internal/platform/opensearch"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"google.golang.org/protobuf/encoding/protojson"
)

type Processor struct {
	store   *store.Store
	engine  *analyticingest.Engine
	indexer platformopensearch.Indexer
}

type Result struct {
	AcceptedEvents  int
	AcceptedSignals int
	CloudSignals    int
	Incidents       int
}

func NewProcessor(st *store.Store, indexer platformopensearch.Indexer) *Processor {
	if indexer == nil {
		indexer = platformopensearch.NoopIndexer{}
	}
	return &Processor{store: st, engine: analyticingest.NewEngine(), indexer: indexer}
}

func (p *Processor) Process(ctx context.Context, batch *analyticsv1.UploadBatch) (Result, error) {
	if p == nil || p.store == nil {
		return Result{}, fmt.Errorf("ingest processor store is nil")
	}
	if batch == nil || batch.GetAgent() == nil {
		return Result{}, fmt.Errorf("upload batch agent identity is required")
	}
	p.store.AddAgent(batch.GetAgent())
	touchedScenarios := map[string]*analyticsv1.AgentHello{}
	acceptedEvents := 0
	acceptedSignals := 0
	acceptedSignalList := []*signalv1.Signal{}
	for _, ev := range batch.GetEvents() {
		inserted := p.store.AddEvent(ev)
		if inserted {
			acceptedEvents++
		}
		if inserted && ev.GetScenario() != "" {
			touchedScenarios[ev.GetScenario()] = batch.GetAgent()
		}
	}
	for _, sig := range batch.GetSignals() {
		inserted := p.store.AddSignal(sig)
		if inserted {
			acceptedSignals++
			acceptedSignalList = append(acceptedSignalList, sig)
		}
		if inserted && sig.GetScenario() != "" {
			touchedScenarios[sig.GetScenario()] = batch.GetAgent()
		}
	}
	start := time.Now()
	p.engine.SetRarityBaseline(p.store.RarityBaselineSnapshot())
	cloudSignals, incidents := p.recomputeTouchedScenarios(touchedScenarios)
	convergenceLatency := time.Since(start)
	p.store.RecordUpload(acceptedEvents, acceptedSignals, cloudSignals, incidents, convergenceLatency)
	p.store.ObserveRaritySignals(acceptedSignalList)
	p.indexSecurityData(ctx, batch, touchedScenarios)
	if err := p.store.Save(); err != nil {
		return Result{}, err
	}
	return Result{AcceptedEvents: acceptedEvents, AcceptedSignals: acceptedSignals, CloudSignals: cloudSignals, Incidents: incidents}, nil
}

func (p *Processor) recomputeTouchedScenarios(touchedScenarios map[string]*analyticsv1.AgentHello) (int, int) {
	totalCloud := 0
	totalIncidents := 0
	for scenario, agent := range touchedScenarios {
		events := p.store.ListEvents(scenario, "")
		endpointSignals := p.store.ListSignals(scenario, "endpoint", false)
		policy := p.effectiveDetectionPolicyForAgent(agent)
		analysis := p.engine.AnalyzeWithPolicy(events, endpointSignals, policy)
		p.store.ReplaceDerivedForScenario(scenario, analysis.CloudSignals, analysis.Incidents)
		totalCloud += len(analysis.CloudSignals)
		totalIncidents += len(analysis.Incidents)
	}
	return totalCloud, totalIncidents
}

func (p *Processor) effectiveDetectionPolicyForAgent(agent *analyticsv1.AgentHello) *policyv1.DetectionPolicy {
	if agent == nil {
		policy, _ := p.store.EffectivePolicy("default", "", "", "")
		return policy.DetectionPolicy()
	}
	var scope agenthealth.RuntimeScope
	if health, ok := p.store.GetAgentHealth(agent.GetTenantId(), agent.GetAgentId()); ok {
		scope = health.Scope
	}
	policy, _ := p.store.EffectivePolicy(agent.GetTenantId(), agent.GetAgentId(), scope.Type, scope.Selector)
	return policy.DetectionPolicy()
}

func (p *Processor) indexSecurityData(ctx context.Context, batch *analyticsv1.UploadBatch, touchedScenarios map[string]*analyticsv1.AgentHello) {
	for _, ev := range batch.GetEvents() {
		if ev.GetId() == "" {
			continue
		}
		raw, err := protojson.Marshal(ev)
		if err == nil {
			_ = p.indexer.Index(ctx, platformopensearch.Document{Index: "sysarmor-events", ID: ev.GetId(), Body: raw})
		}
	}
	for _, sig := range batch.GetSignals() {
		id := SignalDocumentID(sig)
		if id == "" {
			continue
		}
		raw, err := protojson.Marshal(sig)
		if err == nil {
			_ = p.indexer.Index(ctx, platformopensearch.Document{Index: "sysarmor-signals", ID: id, Body: raw})
		}
	}
	for scenario := range touchedScenarios {
		for _, inc := range p.store.ListIncidents(scenario) {
			if inc.GetId() == "" {
				continue
			}
			raw, err := protojson.Marshal(inc)
			if err == nil {
				_ = p.indexer.Index(ctx, platformopensearch.Document{Index: "sysarmor-incidents", ID: inc.GetId(), Body: raw})
				_ = p.indexer.Index(ctx, platformopensearch.Document{Index: "sysarmor-incident-timeline", ID: inc.GetId() + ":state", Body: raw})
			}
			if inc.GetEvidence() != nil {
				evidenceRaw, err := protojson.Marshal(inc.GetEvidence())
				if err == nil {
					_ = p.indexer.Index(ctx, platformopensearch.Document{Index: "sysarmor-evidence", ID: inc.GetId() + ":evidence", Body: evidenceRaw})
				}
			}
		}
	}
}

func SignalDocumentID(sig *signalv1.Signal) string {
	if sig == nil {
		return ""
	}
	if sig.GetId() != "" {
		return sig.GetId()
	}
	parts := []string{sig.GetScenario(), sig.GetName(), sig.GetLineageId()}
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	id := strings.Join(parts, ":")
	if strings.Trim(id, ":") == "" {
		return ""
	}
	return id
}
