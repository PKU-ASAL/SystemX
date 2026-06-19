package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sysarmor/sysarmor-next-project/internal/eventmodel"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
)

type CollectionPolicy struct {
	PolicyID       string                     `json:"policy_id,omitempty"`
	Version        uint64                     `json:"version,omitempty"`
	Behaviors      []string                   `json:"behaviors,omitempty"`
	BehaviorSpecs  []CollectionBehaviorPolicy `json:"-"`
	BinaryPrefixes []string                   `json:"binary_prefixes,omitempty"`
	FilePrefixes   []string                   `json:"file_prefixes,omitempty"`
	SocketFamilies []string                   `json:"socket_families,omitempty"`
	SocketAddrs    []string                   `json:"socket_addrs,omitempty"`
	SocketPorts    []string                   `json:"socket_ports,omitempty"`
	ScopeType      string                     `json:"scope_type,omitempty"`
	ScopeSelector  string                     `json:"scope_selector,omitempty"`
	ObserveOnly    bool                       `json:"observe_only,omitempty"`
}

type CollectionBehaviorPolicy struct {
	ID        string                      `json:"id"`
	Enabled   *bool                       `json:"enabled,omitempty"`
	Selectors CollectionBehaviorSelectors `json:"selectors,omitempty"`
}

type CollectionBehaviorSelectors struct {
	Binary  BinarySelector  `json:"binary,omitempty"`
	Process ProcessSelector `json:"process,omitempty"`
	File    FileSelector    `json:"file,omitempty"`
	Socket  SocketSelector  `json:"socket,omitempty"`
}

type BinarySelector struct {
	Prefixes []string `json:"prefixes,omitempty"`
}

type ProcessSelector struct {
	BinaryPrefixes []string `json:"binary_prefixes,omitempty"`
}

type FileSelector struct {
	Prefixes   []string `json:"prefixes,omitempty"`
	PrefixRefs []string `json:"prefix_refs,omitempty"`
}

type SocketSelector struct {
	Families []string `json:"families,omitempty"`
	Addrs    []string `json:"addrs,omitempty"`
	AddrRefs []string `json:"addr_refs,omitempty"`
	Ports    []string `json:"ports,omitempty"`
	PortRefs []string `json:"port_refs,omitempty"`
}

type CollectionContentSnapshot struct {
	ContextSets map[string]CollectionValueSet
	IOCPacks    map[string]CollectionValueSet
}

type CollectionValueSet struct {
	Ref       string
	Version   string
	Digest    string
	ValueType string
	Values    []string
}

type CollectionExpansionReport struct {
	ResolvedRefs []contract.CollectionResolvedRef
}

const (
	collectionMaxFilePrefixes = 128
	collectionMaxSocketAddrs  = 512
	collectionMaxSocketPorts  = 128
)

func LoadCollectionIntent(path string, observeOnly bool) (contract.CollectionIntent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return contract.CollectionIntent{}, err
	}
	intent, err := ParseCollectionIntent(string(data), observeOnly)
	if err != nil {
		return contract.CollectionIntent{}, fmt.Errorf("%s: %w", path, err)
	}
	return intent, nil
}

func ParseCollectionIntent(data string, observeOnly bool) (contract.CollectionIntent, error) {
	if strings.HasPrefix(strings.TrimSpace(data), "{") {
		policy, err := ParseCollectionPolicyJSON([]byte(data), observeOnly)
		if err != nil {
			return contract.CollectionIntent{}, err
		}
		return CollectionPolicyIntent(policy)
	}
	return contract.CollectionIntent{}, fmt.Errorf("collection policy must be json with behaviors")
}

func ParseCollectionPolicyJSON(data []byte, defaultObserveOnly bool) (CollectionPolicy, error) {
	var policy CollectionPolicy
	var envelope struct {
		Collection json.RawMessage `json:"collection"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return CollectionPolicy{}, fmt.Errorf("decode collection policy: %w", err)
	}
	if len(envelope.Collection) > 0 {
		data = envelope.Collection
	}
	if err := decodeCollectionPolicy(data, &policy); err != nil {
		return CollectionPolicy{}, err
	}
	if !policy.ObserveOnly {
		policy.ObserveOnly = defaultObserveOnly
	}
	return NormalizeCollectionPolicy(policy), nil
}

func decodeCollectionPolicy(data []byte, policy *CollectionPolicy) error {
	var wire struct {
		PolicyID       string          `json:"policy_id,omitempty"`
		Version        uint64          `json:"version,omitempty"`
		Behaviors      json.RawMessage `json:"behaviors,omitempty"`
		BinaryPrefixes []string        `json:"binary_prefixes,omitempty"`
		FilePrefixes   []string        `json:"file_prefixes,omitempty"`
		SocketFamilies []string        `json:"socket_families,omitempty"`
		SocketAddrs    []string        `json:"socket_addrs,omitempty"`
		SocketPorts    []string        `json:"socket_ports,omitempty"`
		ScopeType      string          `json:"scope_type,omitempty"`
		ScopeSelector  string          `json:"scope_selector,omitempty"`
		ObserveOnly    bool            `json:"observe_only,omitempty"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("decode collection policy: %w", err)
	}
	policy.PolicyID = wire.PolicyID
	policy.Version = wire.Version
	policy.BinaryPrefixes = wire.BinaryPrefixes
	policy.FilePrefixes = wire.FilePrefixes
	policy.SocketFamilies = wire.SocketFamilies
	policy.SocketAddrs = wire.SocketAddrs
	policy.SocketPorts = wire.SocketPorts
	policy.ScopeType = wire.ScopeType
	policy.ScopeSelector = wire.ScopeSelector
	policy.ObserveOnly = wire.ObserveOnly
	if len(wire.Behaviors) == 0 {
		return nil
	}
	var behaviorIDs []string
	if err := json.Unmarshal(wire.Behaviors, &behaviorIDs); err == nil {
		policy.Behaviors = behaviorIDs
		return nil
	}
	var behaviorSpecs []CollectionBehaviorPolicy
	if err := json.Unmarshal(wire.Behaviors, &behaviorSpecs); err != nil {
		return fmt.Errorf("decode collection behaviors: %w", err)
	}
	policy.BehaviorSpecs = behaviorSpecs
	return nil
}

func NormalizeCollectionPolicy(policy CollectionPolicy) CollectionPolicy {
	policy.PolicyID = strings.TrimSpace(policy.PolicyID)
	if policy.PolicyID == "" {
		policy.PolicyID = "local-collection-policy"
	}
	if policy.Version == 0 {
		policy.Version = 1
	}
	policy.Behaviors = normalizeBehaviorList(policy.Behaviors)
	policy.BehaviorSpecs = normalizeBehaviorSpecs(policy.BehaviorSpecs)
	policy.BinaryPrefixes = normalizeStringList(policy.BinaryPrefixes)
	policy.FilePrefixes = normalizeStringList(policy.FilePrefixes)
	policy.SocketFamilies = normalizeStringList(policy.SocketFamilies)
	policy.SocketAddrs = normalizeStringList(policy.SocketAddrs)
	policy.SocketPorts = normalizeStringList(policy.SocketPorts)
	policy.ScopeType = strings.TrimSpace(policy.ScopeType)
	policy.ScopeSelector = strings.TrimSpace(policy.ScopeSelector)
	return policy
}

func ExpandCollectionPolicyRefs(policy CollectionPolicy, snapshot CollectionContentSnapshot) (CollectionPolicy, CollectionExpansionReport, error) {
	policy = NormalizeCollectionPolicy(policy)
	var report CollectionExpansionReport
	for specIndex := range policy.BehaviorSpecs {
		spec := &policy.BehaviorSpecs[specIndex]
		behavior := eventmodel.NormalizeBehavior(spec.ID).String()
		filePrefixes, resolved, err := expandSelectorRefs(snapshot, behavior, "file.path.prefix", spec.Selectors.File.PrefixRefs, []string{"path_prefix"}, collectionMaxFilePrefixes)
		if err != nil {
			return CollectionPolicy{}, CollectionExpansionReport{}, err
		}
		spec.Selectors.File.Prefixes = normalizeStringList(append(spec.Selectors.File.Prefixes, filePrefixes...))
		report.ResolvedRefs = append(report.ResolvedRefs, resolved...)
		socketAddrs, resolved, err := expandSelectorRefs(snapshot, behavior, "socket.addr", spec.Selectors.Socket.AddrRefs, []string{"ip", "addr", "ip_addr"}, collectionMaxSocketAddrs)
		if err != nil {
			return CollectionPolicy{}, CollectionExpansionReport{}, err
		}
		spec.Selectors.Socket.Addrs = normalizeStringList(append(spec.Selectors.Socket.Addrs, socketAddrs...))
		report.ResolvedRefs = append(report.ResolvedRefs, resolved...)
		socketPorts, resolved, err := expandSelectorRefs(snapshot, behavior, "socket.port", spec.Selectors.Socket.PortRefs, []string{"port"}, collectionMaxSocketPorts)
		if err != nil {
			return CollectionPolicy{}, CollectionExpansionReport{}, err
		}
		spec.Selectors.Socket.Ports = normalizeStringList(append(spec.Selectors.Socket.Ports, socketPorts...))
		report.ResolvedRefs = append(report.ResolvedRefs, resolved...)
		if len(spec.Selectors.File.Prefixes) > collectionMaxFilePrefixes {
			return CollectionPolicy{}, CollectionExpansionReport{}, fmt.Errorf("collection selector file.path.prefix for %s exceeds budget: %d > %d", behavior, len(spec.Selectors.File.Prefixes), collectionMaxFilePrefixes)
		}
		if len(spec.Selectors.Socket.Addrs) > collectionMaxSocketAddrs {
			return CollectionPolicy{}, CollectionExpansionReport{}, fmt.Errorf("collection selector socket.addr for %s exceeds budget: %d > %d", behavior, len(spec.Selectors.Socket.Addrs), collectionMaxSocketAddrs)
		}
		if len(spec.Selectors.Socket.Ports) > collectionMaxSocketPorts {
			return CollectionPolicy{}, CollectionExpansionReport{}, fmt.Errorf("collection selector socket.port for %s exceeds budget: %d > %d", behavior, len(spec.Selectors.Socket.Ports), collectionMaxSocketPorts)
		}
	}
	return NormalizeCollectionPolicy(policy), report, nil
}

func expandSelectorRefs(snapshot CollectionContentSnapshot, behavior, selector string, refs []string, valueTypes []string, budget int) ([]string, []contract.CollectionResolvedRef, error) {
	refs = normalizeStringList(refs)
	var values []string
	var resolved []contract.CollectionResolvedRef
	for _, ref := range refs {
		set, ok := lookupCollectionValueSet(snapshot, ref)
		if !ok {
			return nil, nil, fmt.Errorf("collection selector %s for %s references missing content %q", selector, behavior, ref)
		}
		if !valueTypeAllowed(set.ValueType, valueTypes) {
			return nil, nil, fmt.Errorf("collection selector %s for %s references %s with value_type %q, want one of %s", selector, behavior, ref, set.ValueType, strings.Join(valueTypes, ","))
		}
		next := normalizeStringList(append(values, set.Values...))
		if len(next) > budget {
			return nil, nil, fmt.Errorf("collection selector %s for %s exceeds budget after %s: %d > %d", selector, behavior, ref, len(next), budget)
		}
		values = next
		resolved = append(resolved, contract.CollectionResolvedRef{
			Behavior:  behavior,
			Selector:  selector,
			Ref:       set.Ref,
			Version:   set.Version,
			Digest:    set.Digest,
			ValueType: set.ValueType,
			Count:     len(set.Values),
			Values:    append([]string(nil), set.Values...),
		})
	}
	return values, resolved, nil
}

func lookupCollectionValueSet(snapshot CollectionContentSnapshot, ref string) (CollectionValueSet, bool) {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "ctx:") {
		set, ok := snapshot.ContextSets[ref]
		return set, ok
	}
	if strings.HasPrefix(ref, "ioc:") {
		set, ok := snapshot.IOCPacks[ref]
		return set, ok
	}
	if set, ok := snapshot.ContextSets[ref]; ok {
		return set, true
	}
	set, ok := snapshot.IOCPacks[ref]
	return set, ok
}

func valueTypeAllowed(valueType string, allowed []string) bool {
	valueType = strings.TrimSpace(valueType)
	for _, candidate := range allowed {
		if valueType == candidate {
			return true
		}
	}
	return false
}

func CollectionPolicyIntent(policy CollectionPolicy) (contract.CollectionIntent, error) {
	policy = NormalizeCollectionPolicy(policy)
	behaviorSeen := map[string]bool{}
	var behaviors []string
	addBehavior := func(behavior string) error {
		normalized := eventmodel.NormalizeBehavior(behavior).String()
		if normalized == "" {
			return nil
		}
		if !eventmodel.KnownBehavior(normalized) {
			return fmt.Errorf("unsupported collection behavior %q", behavior)
		}
		if !behaviorSeen[normalized] {
			behaviorSeen[normalized] = true
			behaviors = append(behaviors, normalized)
		}
		return nil
	}
	for _, behavior := range policy.Behaviors {
		if err := addBehavior(behavior); err != nil {
			return contract.CollectionIntent{}, err
		}
	}
	var behaviorFilters []contract.CollectionBehaviorFilter
	for _, spec := range policy.BehaviorSpecs {
		enabled := true
		if spec.Enabled != nil {
			enabled = *spec.Enabled
		}
		if !enabled {
			continue
		}
		if err := addBehavior(spec.ID); err != nil {
			return contract.CollectionIntent{}, err
		}
		behaviorFilters = append(behaviorFilters, behaviorFilter(spec))
	}
	if len(behaviors) == 0 {
		return contract.CollectionIntent{}, fmt.Errorf("collection policy has no supported behaviors")
	}
	if len(behaviorFilters) == 0 {
		behaviorFilters = behaviorFiltersFromFlatFields(behaviors, policy)
	}
	intent := contract.CollectionIntent{
		Behaviors:       behaviors,
		BinaryPrefixes:  append([]string(nil), policy.BinaryPrefixes...),
		FilePrefixes:    append([]string(nil), policy.FilePrefixes...),
		SocketFamilies:  append([]string(nil), policy.SocketFamilies...),
		SocketAddrs:     append([]string(nil), policy.SocketAddrs...),
		SocketPorts:     append([]string(nil), policy.SocketPorts...),
		BehaviorFilters: behaviorFilters,
		ScopeType:       policy.ScopeType,
		ScopeSelector:   policy.ScopeSelector,
		ObserveOnly:     policy.ObserveOnly,
	}
	return intent.NormalizeScope()
}

func behaviorFilter(spec CollectionBehaviorPolicy) contract.CollectionBehaviorFilter {
	selectors := spec.Selectors
	binaryPrefixes := normalizeStringList(append(append([]string(nil), selectors.Binary.Prefixes...), selectors.Process.BinaryPrefixes...))
	return contract.CollectionBehaviorFilter{
		Behavior:       eventmodel.NormalizeBehavior(spec.ID).String(),
		BinaryPrefixes: binaryPrefixes,
		FilePrefixes:   normalizeStringList(selectors.File.Prefixes),
		SocketFamilies: normalizeStringList(selectors.Socket.Families),
		SocketAddrs:    normalizeStringList(selectors.Socket.Addrs),
		SocketPorts:    normalizeStringList(selectors.Socket.Ports),
	}
}

func behaviorFiltersFromFlatFields(behaviors []string, policy CollectionPolicy) []contract.CollectionBehaviorFilter {
	out := make([]contract.CollectionBehaviorFilter, 0, len(behaviors))
	for _, behavior := range behaviors {
		filter := contract.CollectionBehaviorFilter{Behavior: behavior}
		switch behavior {
		case eventmodel.BehaviorProcessExec.String(), eventmodel.BehaviorProcessFork.String():
			filter.BinaryPrefixes = append([]string(nil), policy.BinaryPrefixes...)
		case eventmodel.BehaviorNetworkConnect.String():
			filter.BinaryPrefixes = append([]string(nil), policy.BinaryPrefixes...)
			filter.SocketFamilies = append([]string(nil), policy.SocketFamilies...)
			filter.SocketAddrs = append([]string(nil), policy.SocketAddrs...)
			filter.SocketPorts = append([]string(nil), policy.SocketPorts...)
		case eventmodel.BehaviorFileOpen.String(), eventmodel.BehaviorFileRead.String(), eventmodel.BehaviorFileWrite.String(), eventmodel.BehaviorFileChmod.String():
			filter.BinaryPrefixes = append([]string(nil), policy.BinaryPrefixes...)
			filter.FilePrefixes = append([]string(nil), policy.FilePrefixes...)
		}
		out = append(out, filter)
	}
	return out
}

func WithScope(intent contract.CollectionIntent, scopeType, scopeSelector string) contract.CollectionIntent {
	intent.ScopeType = strings.TrimSpace(scopeType)
	intent.ScopeSelector = strings.TrimSpace(scopeSelector)
	return intent
}

func normalizeStringList(values []string) []string {
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
	return out
}

func normalizeBehaviorList(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = eventmodel.NormalizeBehavior(value).String()
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func normalizeBehaviorSpecs(values []CollectionBehaviorPolicy) []CollectionBehaviorPolicy {
	out := make([]CollectionBehaviorPolicy, 0, len(values))
	for _, value := range values {
		value.ID = eventmodel.NormalizeBehavior(value.ID).String()
		if value.ID == "" {
			continue
		}
		value.Selectors.Binary.Prefixes = normalizeStringList(value.Selectors.Binary.Prefixes)
		value.Selectors.Process.BinaryPrefixes = normalizeStringList(value.Selectors.Process.BinaryPrefixes)
		value.Selectors.File.Prefixes = normalizeStringList(value.Selectors.File.Prefixes)
		value.Selectors.File.PrefixRefs = normalizeStringList(value.Selectors.File.PrefixRefs)
		value.Selectors.Socket.Families = normalizeStringList(value.Selectors.Socket.Families)
		value.Selectors.Socket.Addrs = normalizeStringList(value.Selectors.Socket.Addrs)
		value.Selectors.Socket.AddrRefs = normalizeStringList(value.Selectors.Socket.AddrRefs)
		value.Selectors.Socket.Ports = normalizeStringList(value.Selectors.Socket.Ports)
		value.Selectors.Socket.PortRefs = normalizeStringList(value.Selectors.Socket.PortRefs)
		out = append(out, value)
	}
	return out
}
