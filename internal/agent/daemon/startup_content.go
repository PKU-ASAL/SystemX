package daemon

import (
	"fmt"
	"strings"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	agentcontent "github.com/sysarmor/sysarmor-next-project/internal/agent/content"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/detection"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
)

func newContentStore(cfg config.Config) (*agentcontent.Store, error) {
	options := agentcontent.Options{
		DefaultDir:  cfg.Content.DefaultPath,
		Dir:         cfg.Content.Path,
		TrustedKeys: parseTrustKeys(cfg.Content.TrustKeys),
	}
	if strings.TrimSpace(options.DefaultDir) != "" {
		return agentcontent.OpenLayered(options)
	}
	return agentcontent.NewStoreWithOptions(options)
}

func (r *AgentRuntime) applyStartupDetection(policy policymodel.Policy) error {
	policy = policymodel.Normalize(policy)
	engine, report := detection.NewWithRuntimeLimits(
		policy.Detection, r.currentCollectionIntent(), r.detectionContentSnapshot(), r.detectionLimits(),
	)
	if report.Status == "rejected" {
		r.setDetectionStatus(policy, report, r.contentStore().Snapshot())
		return fmt.Errorf("%s: %s", report.Message, strings.Join(report.Details, "; "))
	}
	r.setPolicy(policy)
	r.setDetection(engine)
	r.setDetectionStatus(policy, report, r.contentStore().Snapshot())
	return nil
}
