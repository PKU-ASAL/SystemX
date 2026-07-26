package daemon

import (
	"strings"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	agentcontent "github.com/sysarmor/sysarmor-next-project/internal/agent/content"
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
