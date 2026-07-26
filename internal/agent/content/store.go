package content

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Envelope struct {
	APIVersion string          `json:"api_version"`
	Kind       string          `json:"kind"`
	Metadata   Metadata        `json:"metadata"`
	Spec       json.RawMessage `json:"spec"`
	Integrity  Integrity       `json:"integrity,omitempty"`
}

type Metadata struct {
	ID        string `json:"id"`
	Version   string `json:"version"`
	TenantID  string `json:"tenant_id,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	TTL       string `json:"ttl,omitempty"`
}

type Integrity struct {
	DigestAlg    string `json:"digest_alg,omitempty"`
	Digest       string `json:"digest,omitempty"`
	SignatureAlg string `json:"signature_alg,omitempty"`
	KeyID        string `json:"key_id,omitempty"`
	Signature    string `json:"signature,omitempty"`
}

type Options struct {
	Dir         string
	TrustedKeys map[string]ed25519.PublicKey
}

type Record struct {
	Ref     string
	Kind    string
	Version string
	Digest  string
	Signed  bool
	Status  string
	RawJSON string
}

type Snapshot struct {
	RulePacks   map[string]Record
	Rules       []Rule
	ContextSets map[string]ValueSet
	IOCPacks    map[string]ValueSet
}

type ValueSet struct {
	Ref       string
	Version   string
	Digest    string
	ValueType string
	Values    []string
}

type Rule struct {
	RuleID         string
	Version        uint64
	RuleSetRef     string
	Severity       string
	RuntimeType    string
	RuntimeEntry   string
	Expr           RuntimeExpr
	Sequence       RuntimeSequence
	Correlate      RuntimeCorrelate
	Suppression    RuntimeSuppression
	RequiredEvents []RequiredEvent
	ContextRefs    []string
	IOCRefs        []string
	ResponseIntent ResponseIntent
	Terminal       *bool
}

type RuntimeExpr struct {
	Conditions     []RuntimeCondition    `json:"conditions"`
	ConditionGroup *RuntimeConditionNode `json:"condition_group,omitempty"`
}

type RuntimeConditionNode struct {
	All       []RuntimeConditionNode `json:"all,omitempty"`
	Any       []RuntimeConditionNode `json:"any,omitempty"`
	Not       *RuntimeConditionNode  `json:"not,omitempty"`
	Condition *RuntimeCondition      `json:"condition,omitempty"`
}

type RuntimeSequence struct {
	Within string        `json:"within"`
	By     []string      `json:"by"`
	Steps  []RuntimeStep `json:"steps"`
}

type RuntimeCorrelate struct {
	Within string        `json:"within"`
	By     []string      `json:"by"`
	Facts  []RuntimeFact `json:"facts"`
}

type RuntimeFact struct {
	ID             string                `json:"id"`
	Event          string                `json:"event,omitempty"`
	Events         []string              `json:"events,omitempty"`
	Conditions     []RuntimeCondition    `json:"conditions"`
	ConditionGroup *RuntimeConditionNode `json:"condition_group,omitempty"`
}

type RuntimeSuppression struct {
	Within string   `json:"within"`
	By     []string `json:"by"`
}

type RuntimeStep struct {
	ID             string                `json:"id"`
	Behavior       string                `json:"behavior,omitempty"`
	Event          string                `json:"event,omitempty"`
	Conditions     []RuntimeCondition    `json:"conditions"`
	ConditionGroup *RuntimeConditionNode `json:"condition_group,omitempty"`
}

type RuntimeCondition struct {
	Field     string   `json:"field"`
	Op        string   `json:"op"`
	Value     string   `json:"value,omitempty"`
	Values    []string `json:"values,omitempty"`
	Ref       string   `json:"ref,omitempty"`
	Step      string   `json:"step,omitempty"`
	StepField string   `json:"step_field,omitempty"`
}

type RequiredEvent struct {
	Behavior string
	Fields   []string
}

type ResponseIntent struct {
	Action     string
	Confidence uint32
	Reason     string
}

type Store struct {
	mu          sync.RWMutex
	dir         string
	trustedKeys map[string]ed25519.PublicKey
	records     map[string]Record
}

func NewStore() *Store {
	return &Store{records: make(map[string]Record)}
}

func NewStoreWithOptions(opts Options) (*Store, error) {
	store := &Store{dir: strings.TrimSpace(opts.Dir), trustedKeys: opts.TrustedKeys, records: make(map[string]Record)}
	if store.dir == "" {
		return store, nil
	}
	if err := os.MkdirAll(store.dir, 0o755); err != nil {
		return nil, err
	}
	if err := store.Load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Load() error {
	if strings.TrimSpace(s.dir) == "" {
		return nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	records := make(map[string]Record)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return err
		}
		env, err := Parse(string(data))
		if err != nil {
			return fmt.Errorf("load content %s: %w", entry.Name(), err)
		}
		if err := s.Validate(env, false); err != nil {
			return fmt.Errorf("load content %s (%s): %w", entry.Name(), env.Metadata.ID, err)
		}
		if _, exists := records[env.Metadata.ID]; exists {
			return fmt.Errorf("load content %s: duplicate content ref %s", entry.Name(), env.Metadata.ID)
		}
		record := Record{
			Ref:     env.Metadata.ID,
			Kind:    env.Kind,
			Version: env.Metadata.Version,
			Digest:  signedDigest(env),
			Signed:  strings.TrimSpace(env.Integrity.Signature) != "",
			Status:  "loaded",
			RawJSON: string(data),
		}
		if err := validateRecordPayload(record); err != nil {
			return fmt.Errorf("load content %s (%s): %w", entry.Name(), env.Metadata.ID, err)
		}
		records[env.Metadata.ID] = record
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = records
	return nil
}

func validateRecordPayload(record Record) error {
	switch record.Kind {
	case "rulepack":
		if _, err := parseRulePack(record); err != nil {
			return fmt.Errorf("parse rulepack: %w", err)
		}
	case "contextset", "iocpack":
		if _, err := parseValueSet(record); err != nil {
			return fmt.Errorf("parse %s: %w", record.Kind, err)
		}
	}
	return nil
}

func (s *Store) Apply(raw string, allowUnsigned bool, dryRun bool) (Record, error) {
	record, _, err := s.Prepare(raw, allowUnsigned)
	if err != nil {
		return Record{}, err
	}
	if dryRun {
		record.Status = "validated"
		return record, nil
	}
	if err := s.Commit(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *Store) Prepare(raw string, allowUnsigned bool) (Record, Snapshot, error) {
	env, err := Parse(raw)
	if err != nil {
		return Record{}, Snapshot{}, err
	}
	if err := s.Validate(env, allowUnsigned); err != nil {
		return Record{}, Snapshot{}, err
	}
	raw, env, err = s.resolvePatch(env)
	if err != nil {
		return Record{}, Snapshot{}, err
	}
	record := Record{
		Ref:     env.Metadata.ID,
		Kind:    env.Kind,
		Version: env.Metadata.Version,
		Digest:  signedDigest(env),
		Signed:  strings.TrimSpace(env.Integrity.Signature) != "",
		Status:  "applied",
		RawJSON: raw,
	}
	return record, s.SnapshotWith(record), nil
}

func (s *Store) Commit(record Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		s.records = make(map[string]Record)
	}
	s.records[record.Ref] = record
	if err := s.persistLocked(record); err != nil {
		delete(s.records, record.Ref)
		return err
	}
	return nil
}

func (s *Store) List(kind string) []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Record
	for _, record := range s.records {
		if kind != "" && record.Kind != kind {
			continue
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].Ref < out[j].Ref
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func (s *Store) Get(ref string) (Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[ref]
	return record, ok
}

func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return snapshotFromRecords(s.records)
}

func (s *Store) SnapshotWith(record Record) Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	records := make(map[string]Record, len(s.records)+1)
	for ref, current := range s.records {
		records[ref] = current
	}
	if record.Ref != "" {
		records[record.Ref] = record
	}
	return snapshotFromRecords(records)
}

func snapshotFromRecords(records map[string]Record) Snapshot {
	out := Snapshot{
		RulePacks:   make(map[string]Record),
		ContextSets: make(map[string]ValueSet),
		IOCPacks:    make(map[string]ValueSet),
	}
	for _, record := range records {
		switch record.Kind {
		case "rulepack":
			out.RulePacks[record.Ref] = record
			if rules, err := parseRulePack(record); err == nil {
				out.Rules = append(out.Rules, rules...)
			}
		case "contextset":
			if set, err := parseValueSet(record); err == nil {
				out.ContextSets[record.Ref] = set
			}
		case "iocpack":
			if set, err := parseValueSet(record); err == nil {
				out.IOCPacks[record.Ref] = set
			}
		case "content-bundle":
			// Bundle expansion is intentionally deferred; local tests apply
			// individual packages first so each ref has an addressable record.
		}
	}
	return out
}

func Parse(raw string) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return Envelope{}, fmt.Errorf("decode content envelope: %w", err)
	}
	env.Kind = strings.TrimSpace(env.Kind)
	env.Metadata.ID = strings.TrimSpace(env.Metadata.ID)
	env.Metadata.Version = strings.TrimSpace(env.Metadata.Version)
	return env, nil
}

func Validate(env Envelope, raw string, allowUnsigned bool) error {
	return (&Store{}).Validate(env, allowUnsigned)
}

func (s *Store) Validate(env Envelope, allowUnsigned bool) error {
	if env.APIVersion != "sysarmor.content/v1" {
		return fmt.Errorf("unsupported content api_version %q", env.APIVersion)
	}
	switch env.Kind {
	case "rulepack", "contextset", "iocpack", "content-bundle":
	default:
		return fmt.Errorf("unsupported content kind %q", env.Kind)
	}
	if env.Metadata.ID == "" {
		return fmt.Errorf("content metadata.id is required")
	}
	if env.Metadata.Version == "" {
		return fmt.Errorf("content metadata.version is required")
	}
	if len(env.Spec) == 0 || string(env.Spec) == "null" {
		return fmt.Errorf("content spec is required")
	}
	if env.Integrity.Digest != "" {
		if alg := firstNonEmpty(env.Integrity.DigestAlg, "sha256"); alg != "sha256" {
			return fmt.Errorf("unsupported content digest_alg %q", alg)
		}
		if !strings.EqualFold(env.Integrity.Digest, signedDigest(env)) {
			return fmt.Errorf("content digest mismatch")
		}
	}
	if strings.TrimSpace(env.Integrity.Signature) == "" && !allowUnsigned {
		return fmt.Errorf("unsigned content requires allow_unsigned")
	}
	if strings.TrimSpace(env.Integrity.Signature) != "" {
		if err := s.verifySignature(env); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) verifySignature(env Envelope) error {
	if env.Integrity.SignatureAlg != "ed25519" {
		return fmt.Errorf("unsupported content signature_alg %q", env.Integrity.SignatureAlg)
	}
	keyID := strings.TrimSpace(env.Integrity.KeyID)
	if keyID == "" {
		return fmt.Errorf("content signature key_id is required")
	}
	key := s.trustedKeys[keyID]
	if len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("content signature key %q is not trusted", keyID)
	}
	sig, err := base64.StdEncoding.DecodeString(env.Integrity.Signature)
	if err != nil {
		return fmt.Errorf("decode content signature: %w", err)
	}
	if !ed25519.Verify(key, signedBytes(env), sig) {
		return fmt.Errorf("content signature verification failed")
	}
	return nil
}

func signedDigest(env Envelope) string {
	sum := sha256.Sum256(signedBytes(env))
	return hex.EncodeToString(sum[:])
}

func signedBytes(env Envelope) []byte {
	payload := struct {
		APIVersion string          `json:"api_version"`
		Kind       string          `json:"kind"`
		Metadata   Metadata        `json:"metadata"`
		Spec       json.RawMessage `json:"spec"`
	}{
		APIVersion: env.APIVersion,
		Kind:       env.Kind,
		Metadata:   env.Metadata,
		Spec:       env.Spec,
	}
	data, _ := json.Marshal(payload)
	return data
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseValueSet(record Record) (ValueSet, error) {
	env, err := Parse(record.RawJSON)
	if err != nil {
		return ValueSet{}, err
	}
	var spec struct {
		ValueType string   `json:"value_type"`
		Values    []string `json:"values"`
	}
	if err := json.Unmarshal(env.Spec, &spec); err != nil {
		return ValueSet{}, err
	}
	return ValueSet{
		Ref:       record.Ref,
		Version:   record.Version,
		Digest:    record.Digest,
		ValueType: strings.TrimSpace(spec.ValueType),
		Values:    normalizeValues(spec.Values),
	}, nil
}

func normalizeValues(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (s *Store) resolvePatch(env Envelope) (string, Envelope, error) {
	if env.Kind != "contextset" && env.Kind != "iocpack" {
		raw, _ := json.Marshal(env)
		return string(raw), env, nil
	}
	var spec struct {
		BaseVersion   string   `json:"base_version"`
		ValueType     string   `json:"value_type"`
		MergeStrategy string   `json:"merge_strategy"`
		Values        []string `json:"values"`
		Ops           []struct {
			Op    string `json:"op"`
			Value string `json:"value"`
		} `json:"ops"`
	}
	if err := json.Unmarshal(env.Spec, &spec); err != nil {
		return "", Envelope{}, err
	}
	if spec.MergeStrategy != "patch" {
		raw, _ := json.Marshal(env)
		return string(raw), env, nil
	}
	current, ok := s.Get(env.Metadata.ID)
	if !ok {
		return "", Envelope{}, fmt.Errorf("patch base content %s not found", env.Metadata.ID)
	}
	if spec.BaseVersion != "" && current.Version != spec.BaseVersion {
		return "", Envelope{}, fmt.Errorf("patch base_version mismatch: have %s want %s", current.Version, spec.BaseVersion)
	}
	set, err := parseValueSet(current)
	if err != nil {
		return "", Envelope{}, err
	}
	values := map[string]bool{}
	for _, value := range set.Values {
		values[value] = true
	}
	for _, op := range spec.Ops {
		value := strings.TrimSpace(op.Value)
		if value == "" {
			continue
		}
		switch op.Op {
		case "add":
			values[value] = true
		case "remove":
			delete(values, value)
		default:
			return "", Envelope{}, fmt.Errorf("unsupported patch op %q", op.Op)
		}
	}
	var merged []string
	for value := range values {
		merged = append(merged, value)
	}
	sort.Strings(merged)
	resolvedSpec := struct {
		ValueType     string   `json:"value_type,omitempty"`
		MergeStrategy string   `json:"merge_strategy"`
		Values        []string `json:"values"`
	}{
		ValueType:     firstNonEmpty(spec.ValueType, set.ValueType),
		MergeStrategy: "replace",
		Values:        merged,
	}
	specData, err := json.Marshal(resolvedSpec)
	if err != nil {
		return "", Envelope{}, err
	}
	env.Spec = specData
	env.Integrity = Integrity{}
	raw, err := json.Marshal(env)
	if err != nil {
		return "", Envelope{}, err
	}
	return string(raw), env, nil
}

func (s *Store) persistLocked(record Record) error {
	if strings.TrimSpace(s.dir) == "" {
		return nil
	}
	name := safeName(record.Ref) + ".json"
	tmp := filepath.Join(s.dir, name+".tmp")
	dst := filepath.Join(s.dir, name)
	if err := os.WriteFile(tmp, []byte(record.RawJSON), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func safeName(ref string) string {
	replacer := strings.NewReplacer(":", "_", "/", "_", "\\", "_", " ", "_")
	return replacer.Replace(ref)
}

func parseRulePack(record Record) ([]Rule, error) {
	env, err := Parse(record.RawJSON)
	if err != nil {
		return nil, err
	}
	var spec struct {
		RuleSets []struct {
			ID      string `json:"id"`
			Version string `json:"version"`
			Rules   []struct {
				RuleID   string `json:"rule_id"`
				Version  uint64 `json:"version"`
				Severity string `json:"severity"`
				Runtime  struct {
					Type       string           `json:"type"`
					Entrypoint string           `json:"entrypoint"`
					Expr       RuntimeExpr      `json:"expr"`
					Sequence   RuntimeSequence  `json:"sequence"`
					Correlate  RuntimeCorrelate `json:"correlate"`
				} `json:"runtime"`
				Suppress RuntimeSuppression `json:"suppress"`
				Requires struct {
					Events  []RequiredEvent `json:"events"`
					Context struct {
						Required []string `json:"required"`
						Optional []string `json:"optional"`
					} `json:"context"`
					IOC struct {
						Required []string `json:"required"`
						Optional []string `json:"optional"`
					} `json:"ioc"`
				} `json:"requires"`
				Output struct {
					ResponseIntent ResponseIntent `json:"response_intent"`
					Terminal       *bool          `json:"terminal,omitempty"`
				} `json:"output"`
			} `json:"rules"`
		} `json:"rulesets"`
	}
	if err := json.Unmarshal(env.Spec, &spec); err != nil {
		return nil, err
	}
	var out []Rule
	for _, rs := range spec.RuleSets {
		for _, rule := range rs.Rules {
			out = append(out, Rule{
				RuleID:         rule.RuleID,
				Version:        rule.Version,
				RuleSetRef:     rs.ID,
				Severity:       rule.Severity,
				RuntimeType:    rule.Runtime.Type,
				RuntimeEntry:   rule.Runtime.Entrypoint,
				Expr:           rule.Runtime.Expr,
				Sequence:       rule.Runtime.Sequence,
				Correlate:      rule.Runtime.Correlate,
				Suppression:    rule.Suppress,
				RequiredEvents: rule.Requires.Events,
				ContextRefs:    append(append([]string(nil), rule.Requires.Context.Required...), rule.Requires.Context.Optional...),
				IOCRefs:        append(append([]string(nil), rule.Requires.IOC.Required...), rule.Requires.IOC.Optional...),
				ResponseIntent: rule.Output.ResponseIntent,
				Terminal:       rule.Output.Terminal,
			})
		}
	}
	return out, nil
}
