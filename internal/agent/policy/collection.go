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
	PolicyID       string   `json:"policy_id,omitempty"`
	Version        uint64   `json:"version,omitempty"`
	Behaviors      []string `json:"behaviors,omitempty"`
	BinaryPrefixes []string `json:"binary_prefixes,omitempty"`
	FilePrefixes   []string `json:"file_prefixes,omitempty"`
	SocketFamilies []string `json:"socket_families,omitempty"`
	SocketAddrs    []string `json:"socket_addrs,omitempty"`
	SocketPorts    []string `json:"socket_ports,omitempty"`
	ScopeType      string   `json:"scope_type,omitempty"`
	ScopeSelector  string   `json:"scope_selector,omitempty"`
	ObserveOnly    bool     `json:"observe_only,omitempty"`
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
		Collection *CollectionPolicy `json:"collection"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Collection != nil {
		policy = *envelope.Collection
	} else if err := json.Unmarshal(data, &policy); err != nil {
		return CollectionPolicy{}, fmt.Errorf("decode collection policy: %w", err)
	}
	if !policy.ObserveOnly {
		policy.ObserveOnly = defaultObserveOnly
	}
	return NormalizeCollectionPolicy(policy), nil
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
	if len(behaviors) == 0 {
		return contract.CollectionIntent{}, fmt.Errorf("collection policy has no supported behaviors")
	}
	intent := contract.CollectionIntent{
		Behaviors:      behaviors,
		BinaryPrefixes: append([]string(nil), policy.BinaryPrefixes...),
		FilePrefixes:   append([]string(nil), policy.FilePrefixes...),
		SocketFamilies: append([]string(nil), policy.SocketFamilies...),
		SocketAddrs:    append([]string(nil), policy.SocketAddrs...),
		SocketPorts:    append([]string(nil), policy.SocketPorts...),
		ScopeType:      policy.ScopeType,
		ScopeSelector:  policy.ScopeSelector,
		ObserveOnly:    policy.ObserveOnly,
	}
	return intent.NormalizeScope()
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
