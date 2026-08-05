package control

type RequestContext struct {
	RequestID string
	TenantID  string
	AgentID   string
}

type SectionResult struct {
	Name            string
	Status          string
	Message         string
	RequiresRestart bool
	ReportJSON      string
}

type Result struct {
	RequestID       string
	TenantID        string
	AgentID         string
	Status          string
	Message         string
	PolicyID        string
	Version         uint64
	RequiresRestart bool
	Sections        []SectionResult
	Details         []string
	ReportJSON      string
}
