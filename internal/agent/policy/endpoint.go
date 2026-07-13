package policy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/localstore"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
)

const endpointPolicyKind = "endpoint"

type EndpointPolicy = policymodel.EndpointPolicy

func ParseEndpointPolicy(document []byte) (EndpointPolicy, error) {
	var envelope struct {
		PolicyID   string                       `json:"policy_id"`
		Version    uint64                       `json:"version"`
		Collection *CollectionPolicy            `json:"collection"`
		Detection  *policymodel.DetectionPolicy `json:"detection"`
		Telemetry  *policymodel.TelemetryPolicy `json:"telemetry"`
		Response   *responsemodel.Policy        `json:"response"`
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return EndpointPolicy{}, fmt.Errorf("decode endpoint policy: %w", err)
	}
	if strings.TrimSpace(envelope.PolicyID) == "" || envelope.Version == 0 {
		return EndpointPolicy{}, fmt.Errorf("endpoint policy id and positive version are required")
	}
	if envelope.Collection == nil || envelope.Detection == nil || envelope.Telemetry == nil || envelope.Response == nil {
		return EndpointPolicy{}, fmt.Errorf("endpoint policy requires collection, detection, telemetry, and response sections")
	}
	return EndpointPolicy{PolicyID: envelope.PolicyID, Version: envelope.Version, Collection: *envelope.Collection,
		Detection: policymodel.NormalizeDetectionPolicy(*envelope.Detection), Telemetry: *envelope.Telemetry, Response: *envelope.Response}, nil
}

func LoadEffectiveEndpointPolicy(ctx context.Context, store *localstore.Store, path string) (EndpointPolicy, error) {
	if store == nil {
		return EndpointPolicy{}, fmt.Errorf("local store is required")
	}
	if record, ok, err := store.Policy(ctx, endpointPolicyKind); err != nil {
		return EndpointPolicy{}, err
	} else if ok {
		return ParseEndpointPolicy(record.Document)
	}
	document, err := os.ReadFile(path)
	if err != nil {
		return EndpointPolicy{}, fmt.Errorf("read bootstrap policy: %w", err)
	}
	policy, err := ParseEndpointPolicy(document)
	if err != nil {
		return EndpointPolicy{}, err
	}
	if err := SaveEffectiveEndpointPolicy(ctx, store, policy); err != nil {
		return EndpointPolicy{}, fmt.Errorf("persist bootstrap policy: %w", err)
	}
	return policy, nil
}

func SaveEffectiveEndpointPolicy(ctx context.Context, store *localstore.Store, policy EndpointPolicy) error {
	canonical, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	if _, err := ParseEndpointPolicy(canonical); err != nil {
		return err
	}
	digest := sha256.Sum256(canonical)
	if err := store.PutPolicy(ctx, localstore.PolicyRecord{Kind: endpointPolicyKind, Version: policy.Version, Document: canonical, Digest: hex.EncodeToString(digest[:])}); err != nil {
		return err
	}
	return nil
}
