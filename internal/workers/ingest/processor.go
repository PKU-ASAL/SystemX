package ingestworker

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
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

func (p *Processor) Process(ctx context.Context, batch *dataplanev1.DataBatch) (Result, error) {
	if p == nil || p.store == nil {
		return Result{}, fmt.Errorf("ingest processor store is nil")
	}
	if batch == nil || batch.GetHeader() == nil {
		return Result{}, fmt.Errorf("data batch header identity is required")
	}
	agent := store.AgentIdentityFromDataBatch(batch)
	p.store.AddAgent(agent)
	touchedScopes := map[string]touchedScope{}
	acceptedEvents := 0
	acceptedSignals := 0
	acceptedSignalList := []*signalv1.Signal{}
	for _, frame := range batch.GetEvents() {
		ev := frame.GetEvent()
		inserted := p.store.AddEvent(ev)
		if inserted {
			acceptedEvents++
			rememberTouchedScope(touchedScopes, ev.GetLabels(), agent)
		}
	}
	for _, frame := range batch.GetSignals() {
		sig := frame.GetSignal()
		inserted := p.store.AddSignal(sig)
		if inserted {
			acceptedSignals++
			acceptedSignalList = append(acceptedSignalList, sig)
			rememberTouchedScope(touchedScopes, sig.GetLabels(), agent)
		}
	}
	start := time.Now()
	p.engine.SetRarityBaseline(p.store.RarityBaselineSnapshot())
	cloudSignals, incidents := p.recomputeTouchedScopes(ctx, touchedScopes)
	convergenceLatency := time.Since(start)
	p.store.RecordDataBatchIngest(acceptedEvents, acceptedSignals, cloudSignals, incidents, convergenceLatency)
	p.store.ObserveRaritySignals(acceptedSignalList)
	p.indexSecurityData(ctx, batch, touchedScopes)
	if err := p.store.Save(); err != nil {
		return Result{}, err
	}
	if err := p.store.SaveMetrics(); err != nil {
		return Result{}, err
	}
	return Result{AcceptedEvents: acceptedEvents, AcceptedSignals: acceptedSignals, CloudSignals: cloudSignals, Incidents: incidents}, nil
}

type touchedScope struct {
	labels store.LabelSelector
	agent  store.AgentIdentity
}

func rememberTouchedScope(scopes map[string]touchedScope, labels map[string]string, agent store.AgentIdentity) {
	selector := analysisSelector(labels)
	if len(selector) == 0 {
		return
	}
	scopes[labelSelectorKey(selector)] = touchedScope{labels: selector, agent: agent}
}

func analysisSelector(labels map[string]string) store.LabelSelector {
	selector := store.LabelSelector{}
	for _, key := range []string{"case_type", "scenario", "workload"} {
		if value := strings.TrimSpace(labels[key]); value != "" {
			selector[key] = value
		}
	}
	return selector
}

func labelSelectorKey(labels store.LabelSelector) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	return strings.Join(parts, ",")
}

func (p *Processor) recomputeTouchedScopes(ctx context.Context, touchedScopes map[string]touchedScope) (int, int) {
	totalCloud := 0
	totalIncidents := 0
	for _, scope := range touchedScopes {
		events := p.store.ListEvents(scope.labels, "")
		endpointSignals := p.store.ListSignals(scope.labels, "endpoint", false)
		policy := p.effectiveDetectionPolicyForAgent(scope.agent)
		analysis := p.engine.AnalyzeWithPolicy(events, endpointSignals, policy)
		p.store.ReplaceDerivedForLabels(scope.labels, analysis.CloudSignals, analysis.Incidents)
		for _, sig := range analysis.CloudSignals {
			p.indexSignal(ctx, sig)
		}
		totalCloud += len(analysis.CloudSignals)
		totalIncidents += len(analysis.Incidents)
	}
	return totalCloud, totalIncidents
}

func (p *Processor) effectiveDetectionPolicyForAgent(agent store.AgentIdentity) *policyv1.DetectionPolicy {
	agent = agent.Normalized()
	if !agent.Valid() {
		policy, _ := p.store.EffectivePolicy("default", "", "", "")
		return policy.DetectionPolicy()
	}
	var scope agenthealth.RuntimeScope
	if health, ok := p.store.GetAgentHealth(agent.TenantID, agent.AgentID); ok {
		scope = health.Scope
	}
	policy, _ := p.store.EffectivePolicy(agent.TenantID, agent.AgentID, scope.Type, scope.Selector)
	return policy.DetectionPolicy()
}

func (p *Processor) indexSecurityData(ctx context.Context, batch *dataplanev1.DataBatch, touchedScopes map[string]touchedScope) {
	for _, frame := range batch.GetEvents() {
		ev := frame.GetEvent()
		if ev.GetId() == "" {
			continue
		}
		raw, err := protojson.Marshal(ev)
		if err == nil {
			_ = p.indexer.Index(ctx, platformopensearch.Document{Index: "sysarmor-events", ID: ev.GetId(), Body: raw})
		}
	}
	for _, frame := range batch.GetSignals() {
		p.indexSignal(ctx, frame.GetSignal())
	}
	for _, scope := range touchedScopes {
		for _, inc := range p.store.ListIncidents(scope.labels) {
			if inc.GetId() == "" {
				continue
			}
			raw, err := protojson.Marshal(inc)
			if err == nil {
				id := IncidentDocumentID(inc)
				_ = p.indexer.Index(ctx, platformopensearch.Document{Index: "sysarmor-incidents", ID: id, Body: raw})
				_ = p.indexer.Index(ctx, platformopensearch.Document{Index: "sysarmor-incident-timeline", ID: id + ":state", Body: raw})
			}
			if inc.GetEvidence() != nil {
				evidenceRaw, err := protojson.Marshal(inc.GetEvidence())
				if err == nil {
					_ = p.indexer.Index(ctx, platformopensearch.Document{Index: "sysarmor-evidence", ID: IncidentDocumentID(inc) + ":evidence", Body: evidenceRaw})
				}
			}
		}
	}
}

func (p *Processor) indexSignal(ctx context.Context, sig *signalv1.Signal) {
	id := SignalDocumentID(sig)
	if id == "" {
		return
	}
	raw, err := protojson.Marshal(sig)
	if err == nil {
		_ = p.indexer.Index(ctx, platformopensearch.Document{Index: "sysarmor-signals", ID: id, Body: raw})
	}
}

func SignalDocumentID(sig *signalv1.Signal) string {
	if sig == nil {
		return ""
	}
	if sig.GetWhere() == signalv1.SignalWhere_SIGNAL_WHERE_CLOUD {
		return store.SignalProjectionKey(sig)
	}
	if sig.GetId() != "" {
		return sig.GetId()
	}
	parts := []string{labelSelectorKey(analysisSelector(sig.GetLabels())), sig.GetName(), sig.GetLineageId()}
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	id := strings.Join(parts, ":")
	if strings.Trim(id, ":") == "" {
		return ""
	}
	return id
}

func IncidentDocumentID(inc *incidentv1.Incident) string {
	if inc == nil {
		return ""
	}
	if id := store.IncidentProjectionKey(inc); id != "" {
		return id
	}
	return inc.GetId()
}
