package ingestworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
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
	store     *store.Store
	engine    *analyticingest.Engine
	projector platformopensearch.Projector
}

type Result struct {
	AcceptedEvents  int
	AcceptedSignals int
	CloudSignals    int
	Incidents       int
}

func NewProcessor(st *store.Store, projector platformopensearch.Projector) *Processor {
	if projector == nil {
		projector = platformopensearch.NoopIndexer{}
	}
	return &Processor{store: st, engine: analyticingest.NewEngine(), projector: projector}
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
		rememberTouchedScope(touchedScopes, ev.GetLabels(), agent)
		if inserted {
			acceptedEvents++
		}
	}
	for _, frame := range batch.GetSignals() {
		sig := frame.GetSignal()
		inserted := p.store.AddSignal(sig)
		rememberTouchedScope(touchedScopes, sig.GetLabels(), agent)
		if inserted {
			acceptedSignals++
			acceptedSignalList = append(acceptedSignalList, sig)
		}
	}
	start := time.Now()
	p.engine.SetRarityBaseline(p.store.RarityBaselineSnapshot())
	cloudSignals, incidents, derivedDocs, err := p.recomputeTouchedScopes(touchedScopes)
	if err != nil {
		return Result{}, err
	}
	docs, err := batchDocuments(batch)
	if err != nil {
		return Result{}, err
	}
	docs = append(docs, derivedDocs...)
	if err := p.projector.BulkIndex(ctx, docs); err != nil {
		return Result{}, err
	}
	convergenceLatency := time.Since(start)
	p.store.RecordDataBatchIngest(acceptedEvents, acceptedSignals, cloudSignals, incidents, convergenceLatency)
	p.store.ObserveRaritySignals(acceptedSignalList)
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

func (p *Processor) recomputeTouchedScopes(touchedScopes map[string]touchedScope) (int, int, []platformopensearch.Document, error) {
	totalCloud := 0
	totalIncidents := 0
	var documents []platformopensearch.Document
	for _, scope := range touchedScopes {
		events := p.store.ListEvents(scope.labels, "")
		endpointSignals := p.store.ListSignals(scope.labels, "endpoint", false)
		policy := p.effectiveDetectionPolicyForAgent(scope.agent)
		analysis := p.engine.AnalyzeWithPolicy(events, endpointSignals, policy)
		firstObserved, lastObserved := incidentObservedRange(events)
		for _, inc := range analysis.Incidents {
			inc.TenantId = scope.agent.Normalized().TenantID
			inc.CorrelationKey = labelSelectorKey(scope.labels)
			inc.AnalysisVersion = "incident.v1"
			inc.FirstObservedAt = firstObserved
			inc.LastObservedAt = lastObserved
		}
		p.store.ReplaceDerivedForLabels(scope.labels, analysis.CloudSignals, nil)
		for _, sig := range analysis.CloudSignals {
			doc, err := signalDocument(sig)
			if err != nil {
				return 0, 0, nil, err
			}
			documents = append(documents, doc)
		}
		for _, inc := range analysis.Incidents {
			docs, err := incidentDocuments(inc)
			if err != nil {
				return 0, 0, nil, err
			}
			documents = append(documents, docs...)
		}
		totalCloud += len(analysis.CloudSignals)
		totalIncidents += len(analysis.Incidents)
	}
	return totalCloud, totalIncidents, documents, nil
}

func incidentObservedRange(events []*eventv1.CanonicalEvent) (string, string) {
	var first, last uint64
	for _, event := range events {
		observed := event.GetOccurredAtNs()
		if observed == 0 {
			continue
		}
		if first == 0 || observed < first {
			first = observed
		}
		if observed > last {
			last = observed
		}
	}
	if first == 0 {
		return "", ""
	}
	return time.Unix(0, int64(first)).UTC().Format(time.RFC3339Nano), time.Unix(0, int64(last)).UTC().Format(time.RFC3339Nano)
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

func batchDocuments(batch *dataplanev1.DataBatch) ([]platformopensearch.Document, error) {
	var documents []platformopensearch.Document
	for _, frame := range batch.GetEvents() {
		ev := frame.GetEvent()
		if ev.GetId() == "" {
			continue
		}
		raw, err := protojson.Marshal(ev)
		if err != nil {
			return nil, fmt.Errorf("marshal event %q: %w", ev.GetId(), err)
		}
		documents = append(documents, platformopensearch.Document{Index: "sysarmor-events", ID: ev.GetId(), Body: raw})
	}
	for _, frame := range batch.GetSignals() {
		doc, err := signalDocument(frame.GetSignal())
		if err != nil {
			return nil, err
		}
		if doc.ID != "" {
			documents = append(documents, doc)
		}
	}
	return documents, nil
}

func incidentDocuments(inc *incidentv1.Incident) ([]platformopensearch.Document, error) {
	if inc == nil || inc.GetId() == "" {
		return nil, nil
	}
	raw, err := protojson.Marshal(inc)
	if err != nil {
		return nil, fmt.Errorf("marshal incident %q: %w", inc.GetId(), err)
	}
	id := IncidentDocumentID(inc)
	documents := []platformopensearch.Document{}
	if inc.GetEvidence() == nil {
		return append(documents, platformopensearch.Document{Index: "sysarmor-incidents", ID: id, Body: raw}), nil
	}
	evidenceRaw, err := protojson.Marshal(inc.GetEvidence())
	if err != nil {
		return nil, fmt.Errorf("marshal incident evidence %q: %w", id, err)
	}
	documents = append(documents, platformopensearch.Document{Index: "sysarmor-evidence", ID: id + ":evidence", Body: evidenceRaw})
	documents = append(documents, platformopensearch.Document{Index: "sysarmor-incidents", ID: id, Body: raw})
	return documents, nil
}

func signalDocument(sig *signalv1.Signal) (platformopensearch.Document, error) {
	id := SignalDocumentID(sig)
	if id == "" {
		return platformopensearch.Document{}, nil
	}
	raw, err := protojson.Marshal(sig)
	if err != nil {
		return platformopensearch.Document{}, fmt.Errorf("marshal signal %q: %w", id, err)
	}
	return platformopensearch.Document{Index: "sysarmor-signals", ID: id, Body: raw}, nil
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
	identity := strings.Join([]string{inc.GetTenantId(), inc.GetCorrelationKey(), inc.GetAnalysisVersion()}, ":")
	if strings.Trim(identity, ":") == "" {
		labels := inc.GetLabels()
		identity = strings.Join([]string{labels["tenant_id"], labels["correlation_key"], labels["analysis_version"]}, ":")
	}
	if strings.Trim(identity, ":") != "" {
		sum := sha256.Sum256([]byte(identity))
		return "incident:" + hex.EncodeToString(sum[:16])
	}
	return inc.GetId()
}
