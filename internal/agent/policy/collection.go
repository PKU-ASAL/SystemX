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
	Prefixes []string `json:"prefixes,omitempty"`
}

type SocketSelector struct {
	Families []string `json:"families,omitempty"`
	Addrs    []string `json:"addrs,omitempty"`
	Ports    []string `json:"ports,omitempty"`
}

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
		value.Selectors.Socket.Families = normalizeStringList(value.Selectors.Socket.Families)
		value.Selectors.Socket.Addrs = normalizeStringList(value.Selectors.Socket.Addrs)
		value.Selectors.Socket.Ports = normalizeStringList(value.Selectors.Socket.Ports)
		out = append(out, value)
	}
	return out
}
