package detection

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/eventmodel"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
)

const builtinRuleSetRef = "ruleset:endpoint-linux-builtin"
const defaultMaxCEPGroups = 4096
const defaultMaxCEPRefs = 128
const credentialReadSuppressWindow = 5 * time.Minute
const maxSuppressionKeys = 8192

type Engine struct {
	nextID      uint64
	rules       map[string]effectiveRule
	state       map[string]*lineageState
	cep         map[string]*cepRuleState
	cepActive   map[string]map[string]int
	suppression map[string]time.Time
	limits      EngineLimits
	metrics     Metrics
	ctx         ContextSnapshot
	ioc         IOCSnapshot
	refs        ContentSnapshot
	compiled    compiledRuntime
	sequence    compiledSequenceRuntime
}

type EngineLimits struct {
	MaxCEPGroups int
	MaxCEPRefs   int
}

type Metrics struct {
	EventsProcessed     uint64
	EventsByBehavior    map[string]uint64
	CEPRulesScanned     uint64
	CEPRulesEvaluated   uint64
	ConditionsEvaluated uint64
	ConditionsMatched   uint64
	FieldReads          uint64
	ProcessNanosTotal   uint64
	ActiveCEPGroups     uint64
	EvictedCEPGroups    uint64
	ExpiredCEPGroups    uint64
	DroppedEventRefs    uint64
	CEPEvalErrors       uint64
	EmittedSignals      uint64
}

type ContextSnapshot struct {
	CredentialPathPrefixes []string
	PayloadPathPrefixes    []string
	TrustedAdminBinaries   []string
}

type IOCSnapshot struct {
	C2DownloadPorts []string
	C2ControlPorts  []string
	C2Addrs         []string
}

type ContentSnapshot struct {
	ContextRefs map[string]ContentRef
	IOCRefs     map[string]ContentRef
	Rules       []RuleSpec
}

type ContentRef struct {
	Ref     string
	Version string
	Digest  string
	Values  []string
}

type effectiveRule struct {
	spec     RuleSpec
	enabled  bool
	mode     string
	severity string
	intent   *policymodel.ResponseIntentRef
	params   map[string]string
}

type RuleSpec struct {
	RuleID            string
	Version           uint64
	RuleSetRef        string
	Where             string
	Severity          string
	Runtime           string
	RuntimeType       string
	Expr              ExprSpec
	Sequence          SequenceSpec
	RequiredEvents    []RequiredEventSpec
	RequiredBehaviors []string
	ContextRefs       []string
	IOCRefs           []string
	ResponseIntent    *policymodel.ResponseIntentRef
}

type RequiredEventSpec struct {
	Behavior string
	Fields   []string
}

type ExprSpec struct {
	Conditions []ConditionSpec
}

type SequenceSpec struct {
	Within time.Duration
	By     []string
	Steps  []StepSpec
}

type StepSpec struct {
	ID         string
	Behavior   string
	Conditions []ConditionSpec
}

type ConditionSpec struct {
	Field     string
	Op        string
	Value     string
	Values    []string
	Ref       string
	Step      string
	StepField string
}

type cepRuleState struct {
	Groups map[string]*cepGroupState
}

type cepGroupState struct {
	StepIndex       int
	Refs            []string
	Values          map[string]map[string]string
	ExpiresAt       uint64
	WaitingBehavior string
}

type lineageState struct {
	webShellExecRefs   []string
	downloadRefs       []string
	payloadRefs        []string
	payloadExecRefs    []string
	payloads           map[string]bool
	payloadExecStable  map[string]bool
	lastWriterByPath   map[string]string
	stagedPayloadSeen  bool
	reverseShellSeen   bool
	lastExecByStableID map[string]string
}

type ApplyReport struct {
	Status   string         `json:"status"`
	Message  string         `json:"message"`
	Details  []string       `json:"details,omitempty"`
	RuleIDs  []string       `json:"rule_ids,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
	Coverage CoverageReport `json:"coverage,omitempty"`
}

type CoverageReport struct {
	Status   string         `json:"status"`
	Rules    []RuleCoverage `json:"rules,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

type RuleCoverage struct {
	RuleID            string   `json:"rule_id"`
	Status            string   `json:"status"`
	RequiredBehaviors []string `json:"required_behaviors,omitempty"`
	RequiredFields    []string `json:"required_fields,omitempty"`
	MissingBehaviors  []string `json:"missing_behaviors,omitempty"`
	MissingFields     []string `json:"missing_fields,omitempty"`
}

func New(policy *policymodel.DetectionPolicy) (*Engine, ApplyReport) {
	return NewWithInputs(policy, contract.CollectionIntent{})
}

func NewWithInputs(policy *policymodel.DetectionPolicy, collection contract.CollectionIntent) (*Engine, ApplyReport) {
	return NewWithRuntime(policy, collection, ContentSnapshot{})
}

func NewWithRuntime(policy *policymodel.DetectionPolicy, collection contract.CollectionIntent, content ContentSnapshot) (*Engine, ApplyReport) {
	return NewWithRuntimeLimits(policy, collection, content, EngineLimits{})
}

func NewWithRuntimeLimits(policy *policymodel.DetectionPolicy, collection contract.CollectionIntent, content ContentSnapshot, limits EngineLimits) (*Engine, ApplyReport) {
	normalized := policymodel.DefaultDetectionPolicy()
	if policy != nil {
		tmp := policymodel.NormalizeDetectionPolicy(*policy)
		normalized = &tmp
	}
	limits = normalizeLimits(limits)
	engine := &Engine{
		rules:       make(map[string]effectiveRule),
		state:       make(map[string]*lineageState),
		cep:         make(map[string]*cepRuleState),
		cepActive:   make(map[string]map[string]int),
		suppression: make(map[string]time.Time),
		limits:      limits,
		ctx:         resolveContext(normalized.ContextRefs, content),
		ioc:         resolveIOC(normalized.IOCRefs, content),
		refs:        content,
	}
	report := ApplyReport{Status: "applied", Message: "detection policy applied"}
	rules := resolveRules(normalized, content)
	for _, rule := range rules {
		engine.rules[rule.spec.RuleID] = rule
		report.RuleIDs = append(report.RuleIDs, rule.spec.RuleID)
	}
	if errs := validateEffectiveRules(engine.rules); len(errs) > 0 {
		report.Status = "rejected"
		report.Message = "detection policy rejected"
		report.Details = append(report.Details, errs...)
		report.Warnings = append(report.Warnings, errs...)
		return engine, report
	}
	engine.compiled = compileRuntime(rules, content)
	engine.sequence = compileSequenceRuntime(rules, content)
	report.Coverage = CheckCoverageWithContent(normalized, collection, content)
	report.Warnings = append(report.Warnings, report.Coverage.Warnings...)
	if len(report.Warnings) > 0 {
		report.Status = "degraded"
		report.Message = "detection policy applied with missing collection inputs"
		report.Details = append(report.Details, report.Warnings...)
	}
	return engine, report
}

func validateEffectiveRules(rules map[string]effectiveRule) []string {
	var out []string
	for _, rule := range rules {
		runtimeType := rule.runtimeType()
		switch runtimeType {
		case "", "builtin", "expr", "sequence":
		default:
			out = append(out, fmt.Sprintf("rule %s has unsupported runtime type %q", rule.spec.RuleID, runtimeType))
		}
		if runtimeType == "expr" && len(rule.spec.Expr.Conditions) == 0 {
			out = append(out, fmt.Sprintf("rule %s expr runtime requires conditions", rule.spec.RuleID))
		}
		if runtimeType == "sequence" {
			if len(rule.spec.Sequence.Steps) == 0 {
				out = append(out, fmt.Sprintf("rule %s sequence runtime requires steps", rule.spec.RuleID))
			}
			for _, step := range rule.spec.Sequence.Steps {
				if strings.TrimSpace(step.ID) == "" {
					out = append(out, fmt.Sprintf("rule %s sequence step id is required", rule.spec.RuleID))
				}
				if strings.TrimSpace(step.Behavior) == "" {
					out = append(out, fmt.Sprintf("rule %s sequence step %s behavior is required", rule.spec.RuleID, step.ID))
				}
			}
		}
	}
	return out
}

func normalizeLimits(limits EngineLimits) EngineLimits {
	if limits.MaxCEPGroups <= 0 {
		limits.MaxCEPGroups = defaultMaxCEPGroups
	}
	if limits.MaxCEPRefs <= 0 {
		limits.MaxCEPRefs = defaultMaxCEPRefs
	}
	return limits
}

func (e *Engine) Metrics() Metrics {
	if e == nil {
		return Metrics{}
	}
	metrics := e.metrics
	if len(metrics.EventsByBehavior) > 0 {
		metrics.EventsByBehavior = cloneMetricsMap(metrics.EventsByBehavior)
	}
	var active uint64
	for _, ruleState := range e.cep {
		active += uint64(len(ruleState.Groups))
	}
	metrics.ActiveCEPGroups = active
	return metrics
}

func (e *Engine) Process(ev *eventv1.CanonicalEvent) []*signalv1.Signal {
	if e == nil || ev == nil || ev.GetSubjectProc() == nil {
		return nil
	}
	start := time.Now()
	view := newEventView(ev)
	behavior := view.behavior
	e.metrics.EventsProcessed++
	if e.metrics.EventsByBehavior == nil {
		e.metrics.EventsByBehavior = make(map[string]uint64)
	}
	e.metrics.EventsByBehavior[behavior]++
	state := e.lineage(ev.GetLineageId())
	state.remember(ev)
	var out []*signalv1.Signal
	switch behavior {
	case eventmodel.BehaviorProcessExec.String():
		out = append(out, e.detectWebRuntimeShell(ev, state)...)
		out = append(out, e.detectPayloadExec(ev, state)...)
	case eventmodel.BehaviorNetworkConnect.String():
		out = append(out, e.detectDownloadByLOLBin(ev, state)...)
		out = append(out, e.detectReverseShell(ev, state)...)
		out = append(out, e.detectPayloadConnect(ev, state)...)
	case eventmodel.BehaviorFileOpen.String(), eventmodel.BehaviorFileRead.String():
		out = append(out, e.detectCredentialRead(ev)...)
	case eventmodel.BehaviorFileWrite.String(), eventmodel.BehaviorFileChmod.String():
		out = append(out, e.detectPayloadDrop(ev, state)...)
	}
	out = append(out, e.detectCEPRules(view)...)
	out = compact(out)
	e.metrics.EmittedSignals += uint64(len(out))
	e.metrics.ProcessNanosTotal += uint64(time.Since(start).Nanoseconds())
	return out
}

func cloneMetricsMap(in map[string]uint64) map[string]uint64 {
	out := make(map[string]uint64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func eventBehavior(ev *eventv1.CanonicalEvent) string {
	if ev == nil {
		return ""
	}
	if behavior := strings.TrimSpace(ev.GetBehavior()); behavior != "" {
		return strings.ToLower(behavior)
	}
	return ""
}

func (e *Engine) lineage(id string) *lineageState {
	if id == "" {
		id = "unknown"
	}
	st, ok := e.state[id]
	if !ok {
		st = &lineageState{
			payloads:           make(map[string]bool),
			payloadExecStable:  make(map[string]bool),
			lastWriterByPath:   make(map[string]string),
			lastExecByStableID: make(map[string]string),
		}
		e.state[id] = st
	}
	return st
}

func (s *lineageState) remember(ev *eventv1.CanonicalEvent) {
	if ev.GetSubjectProc() == nil {
		return
	}
	stableID := ev.GetSubjectProc().GetStableId()
	if eventBehavior(ev) == eventmodel.BehaviorProcessExec.String() && stableID != "" {
		s.lastExecByStableID[stableID] = ev.GetId()
	}
}

func (e *Engine) detectWebRuntimeShell(ev *eventv1.CanonicalEvent, st *lineageState) []*signalv1.Signal {
	rule, ok := e.rule("web_runtime_spawns_shell")
	if !ok || !isShell(binaryBase(ev)) {
		return nil
	}
	argv := strings.Join(ev.GetSubjectProc().GetArgv(), " ")
	if !looksLikeWebRuntime(ev.GetParentStableId(), ev.GetSubjectProc().GetBinary(), argv) {
		return nil
	}
	st.webShellExecRefs = appendUnique(st.webShellExecRefs, ev.GetId())
	return []*signalv1.Signal{e.signal(ev, rule, []string{ev.GetId()}, false, processEntity(ev))}
}

func (e *Engine) detectPayloadExec(ev *eventv1.CanonicalEvent, st *lineageState) []*signalv1.Signal {
	path := ev.GetSubjectProc().GetBinary()
	if path == "" {
		return nil
	}
	payloadPath := path
	if !st.payloads[payloadPath] && !hasAnyPrefix(payloadPath, e.ctx.PayloadPathPrefixes) {
		payloadPath = payloadPathFromArgv(ev.GetSubjectProc().GetArgv(), e.ctx.PayloadPathPrefixes)
	}
	if payloadPath != "" && (st.payloads[payloadPath] || hasAnyPrefix(payloadPath, e.ctx.PayloadPathPrefixes)) {
		st.payloads[payloadPath] = true
		st.payloadExecStable[ev.GetSubjectProc().GetStableId()] = true
		st.payloadExecRefs = appendUnique(st.payloadExecRefs, ev.GetId())
		if hasAnyPrefix(payloadPath, []string{"/var/lib/app/plugins/"}) {
			st.stagedPayloadSeen = true
		}
	}
	return nil
}

func (e *Engine) detectDownloadByLOLBin(ev *eventv1.CanonicalEvent, st *lineageState) []*signalv1.Signal {
	rule, ok := e.rule("download_by_lolbin")
	if !ok {
		return nil
	}
	bin := binaryBase(ev)
	if bin != "curl" && bin != "wget" {
		return nil
	}
	if !e.ioc.isDownloadSocket(ev.GetObject().GetSocketAddr()) {
		return nil
	}
	st.downloadRefs = appendUnique(st.downloadRefs, ev.GetId())
	return []*signalv1.Signal{e.signal(ev, rule, []string{ev.GetId()}, false, processEntity(ev), socketEntity(ev))}
}

func (e *Engine) detectReverseShell(ev *eventv1.CanonicalEvent, st *lineageState) []*signalv1.Signal {
	rule, ok := e.rule("reverse_shell_pattern")
	if !ok || !isShell(binaryBase(ev)) || !e.ioc.isControlSocket(ev.GetObject().GetSocketAddr()) {
		return nil
	}
	refs := []string{ev.GetId()}
	refs = appendRefs(refs, st.webShellExecRefs...)
	refs = appendRefs(refs, st.downloadRefs...)
	st.reverseShellSeen = true
	return []*signalv1.Signal{e.signal(ev, rule, refs, true, processEntity(ev), socketEntity(ev))}
}

func (e *Engine) detectPayloadConnect(ev *eventv1.CanonicalEvent, st *lineageState) []*signalv1.Signal {
	if !e.ioc.isControlSocket(ev.GetObject().GetSocketAddr()) {
		return nil
	}
	proc := ev.GetSubjectProc()
	argv := strings.Join(proc.GetArgv(), " ")
	payloadProc := st.payloadExecStable[proc.GetStableId()] ||
		st.payloadExecStable[ev.GetParentStableId()] ||
		(len(st.payloadRefs) > 0 && len(st.payloadExecRefs) > 0) ||
		strings.Contains(argv, "helper") ||
		hasAnyPrefix(proc.GetBinary(), e.ctx.PayloadPathPrefixes)
	if !payloadProc {
		return nil
	}
	var out []*signalv1.Signal
	if rule, ok := e.rule("suspicious_exec_connect"); ok {
		sigRefs := appendRefs(nil, st.payloadExecRefs...)
		sigRefs = appendUnique(sigRefs, ev.GetId())
		out = append(out, e.signal(ev, rule, sigRefs, false, processEntity(ev), fileEntity(firstPayloadPath(st), "subject"), socketEntity(ev)))
	}
	refs := appendRefs(nil, st.downloadRefs...)
	refs = appendRefs(refs, st.payloadRefs...)
	refs = appendRefs(refs, st.payloadExecRefs...)
	refs = appendUnique(refs, ev.GetId())
	if len(refs) > 1 {
		if rule, ok := e.rule("payload_lifecycle"); ok {
			out = append(out, e.signal(ev, rule, refs, false, processEntity(ev), fileEntity(firstPayloadPath(st), "subject"), socketEntity(ev)))
		}
	}
	return out
}

func (e *Engine) detectCredentialRead(ev *eventv1.CanonicalEvent) []*signalv1.Signal {
	rule, ok := e.rule("credential_file_read")
	if !ok {
		return nil
	}
	path := ev.GetObject().GetFilePath()
	if !hasAnyPrefix(path, e.ctx.CredentialPathPrefixes) {
		return nil
	}
	if isTrustedBinary(ev.GetSubjectProc().GetBinary(), e.ctx.TrustedAdminBinaries) {
		return nil
	}
	if e.suppressSignal("credential_file_read:"+credentialReadKey(ev, path), eventWallTime(ev), credentialReadSuppressWindow) {
		return nil
	}
	return []*signalv1.Signal{e.signal(ev, rule, []string{ev.GetId()}, false, processEntity(ev), fileEntity(path, "object"))}
}

func credentialReadKey(ev *eventv1.CanonicalEvent, path string) string {
	proc := ev.GetSubjectProc()
	processKey := firstNonEmpty(proc.GetStableId(), proc.GetBinary(), ev.GetLineageId(), "unknown-process")
	return processKey + "|" + path
}

func eventWallTime(ev *eventv1.CanonicalEvent) time.Time {
	if ev.GetOccurredAtNs() > 0 {
		return time.Unix(0, int64(ev.GetOccurredAtNs())).UTC()
	}
	if ev.GetMonoNs() > 0 {
		return time.Unix(0, int64(ev.GetMonoNs())).UTC()
	}
	return time.Now().UTC()
}

func (e *Engine) suppressSignal(key string, now time.Time, window time.Duration) bool {
	if e == nil || key == "" || window <= 0 {
		return false
	}
	if last, ok := e.suppression[key]; ok && now.Sub(last) < window {
		return true
	}
	if len(e.suppression) >= maxSuppressionKeys {
		cutoff := now.Add(-window)
		for got, last := range e.suppression {
			if last.Before(cutoff) {
				delete(e.suppression, got)
			}
		}
	}
	e.suppression[key] = now
	return false
}

func (e *Engine) detectPayloadDrop(ev *eventv1.CanonicalEvent, st *lineageState) []*signalv1.Signal {
	rule, ok := e.rule("payload_dropped")
	if !ok {
		return nil
	}
	path := ev.GetObject().GetFilePath()
	if path == "" || !hasAnyPrefix(path, e.ctx.PayloadPathPrefixes) {
		return nil
	}
	st.payloads[path] = true
	st.payloadRefs = appendUnique(st.payloadRefs, ev.GetId())
	st.lastWriterByPath[path] = ev.GetSubjectProc().GetStableId()
	return []*signalv1.Signal{e.signal(ev, rule, []string{ev.GetId()}, false, processEntity(ev), fileEntity(path, "object"))}
}

func (e *Engine) rule(id string) (effectiveRule, bool) {
	rule, ok := e.rules[id]
	return rule, ok && rule.enabled
}

func (e *Engine) signal(ev *eventv1.CanonicalEvent, rule effectiveRule, refs []string, terminal bool, entities ...*signalv1.EntityRef) *signalv1.Signal {
	refs = appendRefs(nil, refs...)
	if len(refs) == 0 {
		refs = []string{ev.GetId()}
	}
	e.nextID++
	sig := &signalv1.Signal{
		Id:           fmt.Sprintf("sig-%020d", e.nextID),
		Name:         rule.spec.RuleID,
		RuleId:       rule.spec.RuleID,
		RuleVersion:  rule.spec.Version,
		RulesetRef:   rule.spec.RuleSetRef,
		Where:        signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT,
		BaseRisk:     riskForSeverity(rule.severity),
		Severity:     rule.severity,
		Confidence:   confidenceForRule(rule),
		Mode:         rule.mode,
		LocalRarity:  1,
		GlobalRarity: 1,
		LineageId:    ev.GetLineageId(),
		Entities:     entities,
		EventRefs:    refs,
		Terminal:     terminal,
		Labels:       cloneLabels(ev.GetLabels()),
		ContextRefs:  e.signalContentRefs(rule.spec.ContextRefs, e.refs.ContextRefs),
		IocRefs:      e.signalContentRefs(rule.spec.IOCRefs, e.refs.IOCRefs),
	}
	if terminal || rule.intent != nil {
		intent := rule.intent
		if intent == nil {
			intent = rule.spec.ResponseIntent
		}
		if intent != nil && intent.Action != "" {
			sig.ResponseIntent = &signalv1.ResponseIntent{
				ResponseIntent:    intent.Action,
				RecommendedAction: intent.Action,
				Confidence:        intent.Confidence,
				Reason:            intent.Reason,
			}
		}
	}
	if terminal {
		sig.Evidence = &signalv1.EvidenceBundle{
			Id:        "evb-" + sig.GetId(),
			EventRefs: append([]string(nil), refs...),
			RawRefs:   []string{ev.GetRawRef()},
			Entities:  entities,
			Summary:   fmt.Sprintf("rule=%s version=%d ruleset=%s severity=%s", rule.spec.RuleID, rule.spec.Version, rule.spec.RuleSetRef, rule.severity),
		}
	}
	return sig
}

func cloneLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (e *Engine) signalContentRefs(refs []string, resolved map[string]ContentRef) []*signalv1.ContentRef {
	out := make([]*signalv1.ContentRef, 0, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		item, ok := resolved[ref]
		if !ok {
			item = ContentRef{Ref: ref, Version: "builtin"}
		}
		out = append(out, &signalv1.ContentRef{Ref: item.Ref, Version: item.Version, Digest: item.Digest})
	}
	return out
}

func (e *Engine) detectCEPRules(view eventView) []*signalv1.Signal {
	var out []*signalv1.Signal
	for _, rule := range e.compiled.rulesForBehavior(view.behavior) {
		e.metrics.CEPRulesScanned++
		if !rule.rule.enabled {
			continue
		}
		e.metrics.CEPRulesEvaluated++
		switch rule.kind {
		case compiledRuleExpr:
			if e.matchCompiledConditions(view, rule.expr.conditions, nil) {
				out = append(out, e.signal(view.ev, rule.rule, []string{view.eventID}, false, eventEntities(view.ev)...))
			}
		}
	}
	for _, candidate := range e.sequence.candidatesForBehavior(view.behavior, e.cepActive) {
		e.metrics.CEPRulesScanned++
		if !candidate.rule.rule.enabled {
			continue
		}
		e.metrics.CEPRulesEvaluated++
		if sig := e.detectSequenceCandidate(view, candidate); sig != nil {
			out = append(out, sig)
		}
	}
	return out
}

func (r effectiveRule) isCEP() bool {
	switch r.runtimeType() {
	case "expr", "sequence":
		return true
	default:
		return false
	}
}

func (r effectiveRule) runtimeType() string {
	if r.spec.RuntimeType != "" {
		return strings.ToLower(strings.TrimSpace(r.spec.RuntimeType))
	}
	switch strings.ToLower(strings.TrimSpace(r.spec.Runtime)) {
	case "expr", "sequence":
		return strings.ToLower(strings.TrimSpace(r.spec.Runtime))
	default:
		return ""
	}
}

func (e *Engine) detectSequenceRule(view eventView, rule compiledRule) *signalv1.Signal {
	return e.detectSequenceCandidate(view, compiledSequenceCandidate{rule: rule, firstStep: true})
}

func (e *Engine) detectSequenceCandidate(view eventView, candidate compiledSequenceCandidate) *signalv1.Signal {
	rule := candidate.rule
	seq := rule.sequence
	if len(seq.steps) == 0 {
		return nil
	}
	ruleState := e.cep[rule.rule.spec.RuleID]
	if ruleState == nil {
		ruleState = &cepRuleState{Groups: make(map[string]*cepGroupState)}
		e.cep[rule.rule.spec.RuleID] = ruleState
	}
	groupKey := e.compiledSequenceGroupKey(view, seq.by)
	now := view.eventTime()
	st := ruleState.Groups[groupKey]
	if st != nil && st.ExpiresAt > 0 && now > st.ExpiresAt {
		e.deactivateSequenceWait(rule.rule.spec.RuleID, st.WaitingBehavior)
		delete(ruleState.Groups, groupKey)
		e.metrics.ExpiredCEPGroups++
		st = nil
	}
	if !candidate.firstStep {
		if st == nil || st.StepIndex != candidate.stepIndex {
			return nil
		}
	} else if st != nil && st.StepIndex > 0 {
		if !e.matchCompiledStep(view, seq.steps[0], &cepGroupState{Values: make(map[string]map[string]string)}) {
			return nil
		}
		e.deactivateSequenceWait(rule.rule.spec.RuleID, st.WaitingBehavior)
		st.StepIndex = 0
		st.Refs = nil
		st.Values = make(map[string]map[string]string)
		st.WaitingBehavior = ""
	}
	if st == nil {
		e.evictCEPGroups(rule.rule.spec.RuleID, ruleState, now)
		st = &cepGroupState{Values: make(map[string]map[string]string)}
		ruleState.Groups[groupKey] = st
	}
	if st.StepIndex >= len(seq.steps) {
		e.deactivateSequenceWait(rule.rule.spec.RuleID, st.WaitingBehavior)
		st.StepIndex = 0
		st.Refs = nil
		st.Values = make(map[string]map[string]string)
		st.WaitingBehavior = ""
	}
	step := seq.steps[st.StepIndex]
	if !e.matchCompiledStep(view, step, st) {
		return nil
	}
	st.Refs = appendUnique(st.Refs, view.eventID)
	if len(st.Refs) > e.limits.MaxCEPRefs {
		e.metrics.DroppedEventRefs += uint64(len(st.Refs) - e.limits.MaxCEPRefs)
		st.Refs = st.Refs[len(st.Refs)-e.limits.MaxCEPRefs:]
	}
	if st.Values == nil {
		st.Values = make(map[string]map[string]string)
	}
	if saved := eventFieldMapFromView(view, step.saveFields); len(saved) > 0 {
		st.Values[step.id] = saved
	}
	if st.StepIndex == 0 && seq.within > 0 {
		st.ExpiresAt = now + uint64(seq.within.Nanoseconds())
	}
	st.StepIndex++
	if st.StepIndex < len(seq.steps) {
		e.deactivateSequenceWait(rule.rule.spec.RuleID, st.WaitingBehavior)
		st.WaitingBehavior = seq.steps[st.StepIndex].behavior
		e.activateSequenceWait(rule.rule.spec.RuleID, st.WaitingBehavior)
		return nil
	}
	refs := appendRefs(nil, st.Refs...)
	e.deactivateSequenceWait(rule.rule.spec.RuleID, st.WaitingBehavior)
	delete(ruleState.Groups, groupKey)
	return e.signal(view.ev, rule.rule, refs, true, eventEntities(view.ev)...)
}

func (e *Engine) activateSequenceWait(ruleID, behavior string) {
	if ruleID == "" {
		return
	}
	if e.cepActive == nil {
		e.cepActive = make(map[string]map[string]int)
	}
	if e.cepActive[behavior] == nil {
		e.cepActive[behavior] = make(map[string]int)
	}
	e.cepActive[behavior][ruleID]++
}

func (e *Engine) deactivateSequenceWait(ruleID, behavior string) {
	if ruleID == "" || e.cepActive == nil || e.cepActive[behavior] == nil {
		return
	}
	if e.cepActive[behavior][ruleID] <= 1 {
		delete(e.cepActive[behavior], ruleID)
		if len(e.cepActive[behavior]) == 0 {
			delete(e.cepActive, behavior)
		}
		return
	}
	e.cepActive[behavior][ruleID]--
}

func (e *Engine) evictCEPGroups(ruleID string, ruleState *cepRuleState, now uint64) {
	if ruleState == nil || len(ruleState.Groups) < e.limits.MaxCEPGroups {
		return
	}
	for key, group := range ruleState.Groups {
		if group.ExpiresAt > 0 && now > group.ExpiresAt {
			e.deactivateSequenceWait(ruleID, group.WaitingBehavior)
			delete(ruleState.Groups, key)
			e.metrics.ExpiredCEPGroups++
		}
	}
	for len(ruleState.Groups) >= e.limits.MaxCEPGroups {
		for key := range ruleState.Groups {
			e.deactivateSequenceWait(ruleID, ruleState.Groups[key].WaitingBehavior)
			delete(ruleState.Groups, key)
			e.metrics.EvictedCEPGroups++
			break
		}
	}
}

func (e *Engine) matchStep(ev *eventv1.CanonicalEvent, step StepSpec, st *cepGroupState) bool {
	if behavior := strings.TrimSpace(step.Behavior); behavior != "" && eventBehavior(ev) != eventmodel.NormalizeBehavior(behavior).String() {
		return false
	}
	return e.matchConditions(ev, step.Conditions, st)
}

func (e *Engine) matchConditions(ev *eventv1.CanonicalEvent, conditions []ConditionSpec, st *cepGroupState) bool {
	for _, cond := range conditions {
		if !e.matchCondition(ev, cond, st) {
			return false
		}
	}
	return true
}

func (e *Engine) matchCondition(ev *eventv1.CanonicalEvent, cond ConditionSpec, st *cepGroupState) bool {
	e.metrics.ConditionsEvaluated++
	actual := eventField(ev, cond.Field)
	e.metrics.FieldReads++
	values := append([]string(nil), cond.Values...)
	if cond.Value != "" {
		values = append(values, cond.Value)
	}
	if cond.Ref != "" {
		values = append(values, e.contentValues(cond.Ref)...)
	}
	op := strings.ToLower(strings.TrimSpace(cond.Op))
	if op == "" {
		op = "eq"
	}
	switch op {
	case "eq", "equals":
		return e.recordConditionResult(containsString(values, actual))
	case "neq", "not_eq":
		return e.recordConditionResult(!containsString(values, actual))
	case "contains":
		for _, value := range values {
			if value != "" && strings.Contains(actual, value) {
				return e.recordConditionResult(true)
			}
		}
		return e.recordConditionResult(false)
	case "prefix", "has_prefix":
		for _, value := range values {
			if value != "" && strings.HasPrefix(actual, value) {
				return e.recordConditionResult(true)
			}
		}
		return e.recordConditionResult(false)
	case "suffix", "has_suffix":
		for _, value := range values {
			if value != "" && strings.HasSuffix(actual, value) {
				return e.recordConditionResult(true)
			}
		}
		return e.recordConditionResult(false)
	case "in":
		return e.recordConditionResult(containsString(values, actual))
	case "not_in":
		return e.recordConditionResult(!containsString(values, actual))
	case "same_as":
		if st == nil || cond.Step == "" {
			return e.recordConditionResult(false)
		}
		stepValues := st.Values[cond.Step]
		if stepValues == nil {
			return e.recordConditionResult(false)
		}
		stepField := firstNonEmpty(cond.StepField, cond.Field)
		return e.recordConditionResult(actual != "" && actual == stepValues[stepField])
	case "exists":
		return e.recordConditionResult(actual != "")
	case "gt", "gte", "lt", "lte":
		return e.recordConditionResult(compareNumber(actual, firstValue(values), op))
	default:
		return e.recordConditionResult(false)
	}
}

func (e *Engine) recordConditionResult(matched bool) bool {
	if matched {
		e.metrics.ConditionsMatched++
	}
	return matched
}

func (e *Engine) contentValues(ref string) []string {
	if item, ok := e.refs.ContextRefs[ref]; ok {
		return item.Values
	}
	if item, ok := e.refs.IOCRefs[ref]; ok {
		return item.Values
	}
	return nil
}

func (e *Engine) sequenceGroupKey(ev *eventv1.CanonicalEvent, fields []string) string {
	if len(fields) == 0 {
		fields = []string{"lineage_id"}
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, strings.TrimSpace(field)+"="+eventField(ev, field))
	}
	return strings.Join(parts, "|")
}

func eventTime(ev *eventv1.CanonicalEvent) uint64 {
	if ev.GetOccurredAtNs() != 0 {
		return ev.GetOccurredAtNs()
	}
	return ev.GetMonoNs()
}

func eventFieldMap(ev *eventv1.CanonicalEvent) map[string]string {
	fields := []string{
		"event.id", "event.kind", "lineage_id", "process.stable_id", "process.binary",
		"process.argv", "process.uid", "parent.stable_id", "file.path", "socket.addr",
		"socket.port", "scope.type", "scope.selector", "container.id", "cgroup",
	}
	out := make(map[string]string, len(fields))
	for _, field := range fields {
		out[field] = eventField(ev, field)
	}
	return out
}

func eventField(ev *eventv1.CanonicalEvent, field string) string {
	if ev == nil {
		return ""
	}
	field = strings.TrimSpace(field)
	switch field {
	case "event.id", "id":
		return ev.GetId()
	case "event.behavior", "behavior":
		return ev.GetBehavior()
	case "lineage_id", "lineage.id":
		return ev.GetLineageId()
	case "process.stable_id", "process.id":
		return ev.GetSubjectProc().GetStableId()
	case "process.binary", "binary":
		return ev.GetSubjectProc().GetBinary()
	case "process.argv", "argv":
		return strings.Join(ev.GetSubjectProc().GetArgv(), " ")
	case "process.uid", "uid":
		return strconv.FormatUint(uint64(ev.GetSubjectProc().GetUid()), 10)
	case "parent.stable_id", "parent.id":
		return ev.GetParentStableId()
	case "file.path", "object.file_path":
		return ev.GetObject().GetFilePath()
	case "socket.addr", "object.socket_addr":
		addr, _, ok := strings.Cut(ev.GetObject().GetSocketAddr(), ":")
		if ok {
			return addr
		}
		return ev.GetObject().GetSocketAddr()
	case "socket.port":
		_, port, ok := strings.Cut(ev.GetObject().GetSocketAddr(), ":")
		if ok {
			return port
		}
		return ""
	case "socket":
		return ev.GetObject().GetSocketAddr()
	case "scope.type":
		return ev.GetScope().GetType()
	case "scope.selector":
		return ev.GetScope().GetSelector()
	case "container.id", "container_id":
		return ev.GetContainerId()
	case "cgroup":
		return ev.GetCgroup()
	default:
		return ""
	}
}

func eventEntities(ev *eventv1.CanonicalEvent) []*signalv1.EntityRef {
	entities := []*signalv1.EntityRef{processEntity(ev)}
	if path := ev.GetObject().GetFilePath(); path != "" {
		entities = append(entities, fileEntity(path, "object"))
	}
	if socket := ev.GetObject().GetSocketAddr(); socket != "" {
		entities = append(entities, socketEntity(ev))
	}
	if containerID := ev.GetContainerId(); containerID != "" {
		entities = append(entities, &signalv1.EntityRef{Kind: "container", Key: containerID, Role: "scope"})
	}
	return entities
}

func containsString(values []string, actual string) bool {
	for _, value := range values {
		if actual == value {
			return true
		}
	}
	return false
}

func firstValue(values []string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func compareNumber(actual, expected, op string) bool {
	a, err := strconv.ParseFloat(actual, 64)
	if err != nil {
		return false
	}
	b, err := strconv.ParseFloat(expected, 64)
	if err != nil {
		return false
	}
	switch op {
	case "gt":
		return a > b
	case "gte":
		return a >= b
	case "lt":
		return a < b
	case "lte":
		return a <= b
	default:
		return false
	}
}

func confidenceForRule(rule effectiveRule) uint32 {
	if rule.intent != nil && rule.intent.Confidence > 0 {
		return rule.intent.Confidence
	}
	if rule.spec.ResponseIntent != nil && rule.spec.ResponseIntent.Confidence > 0 {
		return rule.spec.ResponseIntent.Confidence
	}
	switch strings.ToLower(rule.severity) {
	case "critical":
		return 80
	case "high":
		return 70
	case "medium":
		return 55
	default:
		return 40
	}
}

func resolveRules(policy *policymodel.DetectionPolicy, content ContentSnapshot) []effectiveRule {
	specs := append(builtinRules(), content.Rules...)
	enabledRuleSets := map[string]bool{}
	for _, ref := range policy.RuleSets {
		if ref.Ref == "" {
			continue
		}
		enabled := true
		if ref.Enabled != nil {
			enabled = *ref.Enabled
		}
		enabledRuleSets[ref.Ref] = enabled
	}
	if len(enabledRuleSets) == 0 {
		enabledRuleSets[builtinRuleSetRef] = true
	}
	overrides := map[string]policymodel.RuleOverride{}
	for _, override := range policy.RuleOverrides {
		if override.RuleID != "" {
			overrides[override.RuleID] = override
		}
	}
	var out []effectiveRule
	for _, spec := range specs {
		if !enabledRuleSets[spec.RuleSetRef] {
			continue
		}
		rule := effectiveRule{
			spec:     spec,
			enabled:  true,
			mode:     firstNonEmpty(policy.Mode, "observe"),
			severity: spec.Severity,
			intent:   spec.ResponseIntent,
		}
		if override, ok := overrides[spec.RuleID]; ok {
			if override.Enabled != nil {
				rule.enabled = *override.Enabled
			}
			if override.Mode != "" {
				rule.mode = override.Mode
			}
			if override.Severity != "" {
				rule.severity = override.Severity
			}
			if override.ResponseIntent != nil {
				rule.intent = override.ResponseIntent
			}
			rule.params = override.Params
		}
		if rule.enabled {
			out = append(out, rule)
		}
	}
	return out
}

func builtinRules() []RuleSpec {
	collect := &policymodel.ResponseIntentRef{Action: "collect_evidence", Confidence: 80, Reason: "terminal endpoint signal"}
	return []RuleSpec{
		{RuleID: "web_runtime_spawns_shell", Version: 1, RuleSetRef: builtinRuleSetRef, Where: "endpoint", Severity: "high", Runtime: "builtin", RequiredBehaviors: []string{eventmodel.BehaviorProcessExec.String()}},
		{RuleID: "download_by_lolbin", Version: 1, RuleSetRef: builtinRuleSetRef, Where: "endpoint", Severity: "medium", Runtime: "builtin", RequiredBehaviors: []string{eventmodel.BehaviorNetworkConnect.String()}, IOCRefs: []string{"ioc:c2-download-port-feed"}},
		{RuleID: "payload_dropped", Version: 1, RuleSetRef: builtinRuleSetRef, Where: "endpoint", Severity: "high", Runtime: "builtin", RequiredBehaviors: []string{eventmodel.BehaviorFileWrite.String(), eventmodel.BehaviorFileChmod.String()}, ContextRefs: []string{"ctx:payload-path-prefixes"}},
		{RuleID: "reverse_shell_pattern", Version: 1, RuleSetRef: builtinRuleSetRef, Where: "endpoint", Severity: "critical", Runtime: "builtin", RequiredBehaviors: []string{eventmodel.BehaviorProcessExec.String(), eventmodel.BehaviorNetworkConnect.String()}, IOCRefs: []string{"ioc:c2-control-port-feed"}, ResponseIntent: collect},
		{RuleID: "suspicious_exec_connect", Version: 1, RuleSetRef: builtinRuleSetRef, Where: "endpoint", Severity: "high", Runtime: "builtin", RequiredBehaviors: []string{eventmodel.BehaviorProcessExec.String(), eventmodel.BehaviorNetworkConnect.String()}, IOCRefs: []string{"ioc:c2-control-port-feed"}},
		{RuleID: "payload_lifecycle", Version: 1, RuleSetRef: builtinRuleSetRef, Where: "endpoint", Severity: "high", Runtime: "builtin", RequiredBehaviors: []string{eventmodel.BehaviorFileWrite.String(), eventmodel.BehaviorProcessExec.String(), eventmodel.BehaviorNetworkConnect.String()}, ContextRefs: []string{"ctx:payload-path-prefixes"}, IOCRefs: []string{"ioc:c2-control-port-feed"}},
		{RuleID: "credential_file_read", Version: 1, RuleSetRef: builtinRuleSetRef, Where: "endpoint", Severity: "medium", Runtime: "builtin", RequiredBehaviors: []string{eventmodel.BehaviorFileRead.String()}, ContextRefs: []string{"ctx:credential-path-prefixes", "ctx:trusted-admin-binaries"}},
	}
}

func CheckDependencies(policy *policymodel.DetectionPolicy, collection contract.CollectionIntent) []string {
	return CheckDependenciesWithContent(policy, collection, ContentSnapshot{})
}

func CheckDependenciesWithContent(policy *policymodel.DetectionPolicy, collection contract.CollectionIntent, content ContentSnapshot) []string {
	return CheckCoverageWithContent(policy, collection, content).Warnings
}

func CheckCoverage(policy *policymodel.DetectionPolicy, collection contract.CollectionIntent) CoverageReport {
	return CheckCoverageWithContent(policy, collection, ContentSnapshot{})
}

func CheckCoverageWithContent(policy *policymodel.DetectionPolicy, collection contract.CollectionIntent, content ContentSnapshot) CoverageReport {
	if policy == nil {
		tmp := policymodel.DefaultDetectionPolicy()
		policy = tmp
	}
	collectedBehaviors := map[string]bool{}
	for _, behavior := range collection.Behaviors {
		behavior = eventmodel.NormalizeBehavior(behavior).String()
		if behavior != "" {
			collectedBehaviors[behavior] = true
		}
	}
	if len(collectedBehaviors) == 0 {
		return CoverageReport{Status: "unknown"}
	}
	report := CoverageReport{Status: "covered"}
	for _, rule := range resolveRules(policy, content) {
		coverage := RuleCoverage{RuleID: rule.spec.RuleID, Status: "covered"}
		for _, behavior := range rule.spec.RequiredBehaviors {
			behavior = eventmodel.NormalizeBehavior(behavior).String()
			if behavior == "" {
				continue
			}
			coverage.RequiredBehaviors = appendUnique(coverage.RequiredBehaviors, behavior)
			if !collectedBehaviors[behavior] {
				coverage.MissingBehaviors = appendUnique(coverage.MissingBehaviors, behavior)
			}
		}
		availableFields := availableFieldsForCollection(collection)
		for _, req := range rule.spec.RequiredEvents {
			behavior := eventmodel.NormalizeBehavior(req.Behavior).String()
			if behavior != "" {
				coverage.RequiredBehaviors = appendUnique(coverage.RequiredBehaviors, behavior)
				if !collectedBehaviors[behavior] {
					coverage.MissingBehaviors = appendUnique(coverage.MissingBehaviors, behavior)
				}
			}
			for _, field := range req.Fields {
				if field == "" {
					continue
				}
				requiredField := fmt.Sprintf("%s:%s", firstNonEmpty(behavior, req.Behavior, "event"), field)
				coverage.RequiredFields = appendUnique(coverage.RequiredFields, requiredField)
				if !availableFields[field] {
					coverage.MissingFields = appendUnique(coverage.MissingFields, requiredField)
				}
			}
		}
		if len(coverage.MissingBehaviors) > 0 || len(coverage.MissingFields) > 0 {
			coverage.Status = "missing_inputs"
			report.Status = "degraded"
			missing := append([]string(nil), coverage.MissingBehaviors...)
			missing = append(missing, coverage.MissingFields...)
			report.Warnings = append(report.Warnings, fmt.Sprintf("rule %s missing collection inputs: %s", rule.spec.RuleID, strings.Join(missing, ",")))
		}
		report.Rules = append(report.Rules, coverage)
	}
	return report
}

func availableFieldsForCollection(collection contract.CollectionIntent) map[string]bool {
	if len(collection.Capabilities) > 0 {
		return availableFieldsFromCapabilities(collection)
	}
	fields := map[string]bool{
		"event.id":       true,
		"event.behavior": true,
		"lineage_id":     true,
		"scope.type":     true,
		"scope.selector": true,
		"container.id":   true,
		"container_id":   true,
		"cgroup":         true,
	}
	addProcess := func() {
		for _, field := range []string{"process.stable_id", "process.id", "process.binary", "process.argv", "process.uid", "parent.stable_id", "parent.id"} {
			fields[field] = true
		}
	}
	if len(collection.Behaviors) == 0 {
		for _, field := range []string{"file.path", "object.file_path", "socket.addr", "object.socket_addr", "socket.port", "socket"} {
			fields[field] = true
		}
		addProcess()
		return fields
	}
	behaviors := map[string]bool{}
	for _, behavior := range collection.Behaviors {
		behaviors[eventmodel.NormalizeBehavior(behavior).String()] = true
	}
	if behaviors[eventmodel.BehaviorProcessExec.String()] || behaviors[eventmodel.BehaviorProcessFork.String()] || behaviors[eventmodel.BehaviorProcessExit.String()] {
		addProcess()
	}
	if behaviors[eventmodel.BehaviorFileOpen.String()] || behaviors[eventmodel.BehaviorFileRead.String()] || behaviors[eventmodel.BehaviorFileWrite.String()] || behaviors[eventmodel.BehaviorFileChmod.String()] {
		addProcess()
		fields["file.path"] = true
		fields["object.file_path"] = true
	}
	if behaviors[eventmodel.BehaviorNetworkConnect.String()] {
		addProcess()
		fields["socket.addr"] = true
		fields["object.socket_addr"] = true
		fields["socket.port"] = true
		fields["socket"] = true
	}
	return fields
}

func availableFieldsFromCapabilities(collection contract.CollectionIntent) map[string]bool {
	fields := map[string]bool{}
	behaviors := map[string]bool{}
	for _, behavior := range collection.Behaviors {
		behavior = eventmodel.NormalizeBehavior(behavior).String()
		if behavior != "" {
			behaviors[behavior] = true
		}
	}
	for _, behavior := range collection.Capabilities {
		if len(behaviors) > 0 && !behaviors[eventmodel.NormalizeBehavior(behavior.Behavior).String()] {
			continue
		}
		for _, field := range behavior.Fields {
			if field != "" {
				fields[field] = true
			}
		}
	}
	return fields
}

func resolveContext(refs []policymodel.ContentRef, content ContentSnapshot) ContextSnapshot {
	out := ContextSnapshot{
		CredentialPathPrefixes: []string{"/root/.ssh/", "/home/", "/var/run/secrets/", "/run/secrets/", "/etc/kubernetes/"},
		PayloadPathPrefixes:    []string{"/tmp/", "/dev/shm/", "/var/lib/app/plugins/"},
		TrustedAdminBinaries:   []string{"/usr/bin/vim", "/usr/bin/vi", "/usr/bin/nano"},
	}
	for _, ref := range refs {
		item, ok := content.ContextRefs[ref.Ref]
		if !ok {
			continue
		}
		switch ref.Ref {
		case "ctx:credential-path-prefixes":
			out.CredentialPathPrefixes = append([]string(nil), item.Values...)
		case "ctx:payload-path-prefixes":
			out.PayloadPathPrefixes = append([]string(nil), item.Values...)
		case "ctx:trusted-admin-binaries":
			out.TrustedAdminBinaries = append([]string(nil), item.Values...)
		}
	}
	return out
}

func resolveIOC(refs []policymodel.ContentRef, content ContentSnapshot) IOCSnapshot {
	out := IOCSnapshot{
		C2DownloadPorts: []string{"8080"},
		C2ControlPorts:  []string{"443", "8443"},
	}
	for _, ref := range refs {
		item, ok := content.IOCRefs[ref.Ref]
		if !ok {
			continue
		}
		switch ref.Ref {
		case "ioc:c2-port-feed":
			out.C2ControlPorts = append([]string(nil), item.Values...)
		case "ioc:c2-download-port-feed":
			out.C2DownloadPorts = append([]string(nil), item.Values...)
		case "ioc:c2-control-port-feed":
			out.C2ControlPorts = append([]string(nil), item.Values...)
		case "ioc:c2-ip-feed":
			out.C2Addrs = append([]string(nil), item.Values...)
		}
	}
	return out
}

func (i IOCSnapshot) isDownloadSocket(socket string) bool {
	return i.socketMatches(socket, i.C2DownloadPorts)
}

func (i IOCSnapshot) isControlSocket(socket string) bool {
	return i.socketMatches(socket, i.C2ControlPorts)
}

func (i IOCSnapshot) socketMatches(socket string, ports []string) bool {
	addr, port, ok := strings.Cut(socket, ":")
	if !ok {
		return false
	}
	portMatched := false
	for _, candidate := range ports {
		if port == candidate {
			portMatched = true
			break
		}
	}
	if !portMatched {
		return false
	}
	if len(i.C2Addrs) == 0 {
		return true
	}
	for _, candidate := range i.C2Addrs {
		if addr == candidate {
			return true
		}
	}
	return false
}

func processEntity(ev *eventv1.CanonicalEvent) *signalv1.EntityRef {
	key := ""
	if ev.GetSubjectProc() != nil {
		key = ev.GetSubjectProc().GetStableId()
	}
	return &signalv1.EntityRef{Kind: "process", Key: key, Role: "subject"}
}

func socketEntity(ev *eventv1.CanonicalEvent) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "socket", Key: ev.GetObject().GetSocketAddr(), Role: "object"}
}

func fileEntity(path, role string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "file", Key: path, Role: role}
}

func binaryBase(ev *eventv1.CanonicalEvent) string {
	return filepath.Base(ev.GetSubjectProc().GetBinary())
}

func isShell(bin string) bool {
	switch bin {
	case "sh", "bash", "dash", "zsh", "ksh", "ash":
		return true
	default:
		return false
	}
}

func looksLikeWebRuntime(parentStableID, binary, argv string) bool {
	text := strings.ToLower(parentStableID + " " + binary + " " + argv)
	for _, token := range []string{"nginx", "apache", "httpd", "php-fpm", "gunicorn", "uwsgi", "tomcat", "node"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return parentStableID == "parent"
}

func isTrustedBinary(path string, trusted []string) bool {
	for _, candidate := range trusted {
		if path == candidate {
			return true
		}
	}
	return false
}

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func payloadPathFromArgv(argv []string, prefixes []string) string {
	for _, arg := range argv {
		arg = strings.Trim(arg, `"'`)
		if hasAnyPrefix(arg, prefixes) {
			return arg
		}
	}
	return ""
}

func firstPayloadPath(st *lineageState) string {
	for path := range st.payloads {
		return path
	}
	return "/var/lib/app/plugins/helper"
}

func riskForSeverity(severity string) uint32 {
	switch strings.ToLower(severity) {
	case "critical":
		return 80
	case "high":
		return 55
	case "medium":
		return 35
	case "low":
		return 15
	default:
		return 30
	}
}

func appendRefs(base []string, refs ...string) []string {
	out := append([]string(nil), base...)
	for _, ref := range refs {
		out = appendUnique(out, ref)
	}
	return out
}

func appendUnique(items []string, item string) []string {
	if item == "" {
		return items
	}
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}

func compact(in []*signalv1.Signal) []*signalv1.Signal {
	out := in[:0]
	for _, sig := range in {
		if sig != nil {
			out = append(out, sig)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
